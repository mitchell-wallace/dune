#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
FIREWALL_DOMAIN_CONFIG="${DUNE_FIREWALL_DOMAIN_CONFIG:-/usr/local/lib/dune/runtime/firewall-domains.tsv}"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi

if [ ! -f "$FIREWALL_DOMAIN_CONFIG" ]; then
  FIREWALL_DOMAIN_CONFIG="${SCRIPT_DIR}/firewall-domains.tsv"
fi

. "$UTILS_PATH"

DEBUG_FIREWALL="${DUNE_FIREWALL_DEBUG:-0}"
FIREWALL_DEBUG_ENABLED=0
case "$(printf '%s' "$DEBUG_FIREWALL" | tr '[:upper:]' '[:lower:]')" in
  1|true|yes|on)
    FIREWALL_DEBUG_ENABLED=1
    ;;
esac

FIREWALL_RUNTIME_DIR="/run/dune"
FIREWALL_REFRESH_PID_FILE="${FIREWALL_RUNTIME_DIR}/firewall-refresh.pid"
FIREWALL_REFRESH_DOMAINS_FILE="${FIREWALL_RUNTIME_DIR}/firewall-refresh-domains.tsv"
FIREWALL_REFRESH_LOG_FILE="${FIREWALL_RUNTIME_DIR}/firewall-refresh.log"
FIREWALL_REFRESH_INTERVAL_SECONDS="${DUNE_FIREWALL_REFRESH_INTERVAL_SECONDS:-10}"
FIREWALL_REFRESH_ATTEMPTS="${DUNE_FIREWALL_REFRESH_ATTEMPTS:-3}"
FIREWALL_REFRESH_RETRY_DELAY_SECONDS="${DUNE_FIREWALL_REFRESH_RETRY_DELAY_SECONDS:-1}"

log_info() {
  echo "[init-firewall] $*"
}

log_warn() {
  echo "[init-firewall] WARNING: $*" >&2
}

log_error() {
  echo "[init-firewall] ERROR: $*" >&2
}

log_debug() {
  if [ "$FIREWALL_DEBUG_ENABLED" -eq 1 ]; then
    echo "[init-firewall] DEBUG: $*"
  fi
}

validate_positive_int() {
  local value="$1"
  local name="$2"

  if [[ ! "$value" =~ ^[1-9][0-9]*$ ]]; then
    log_error "Invalid ${name}='${value}' (expected positive integer)"
    exit 1
  fi
}

validate_nonnegative_int() {
  local value="$1"
  local name="$2"

  if [[ ! "$value" =~ ^[0-9]+$ ]]; then
    log_error "Invalid ${name}='${value}' (expected non-negative integer)"
    exit 1
  fi
}

validate_refresh_config() {
  validate_nonnegative_int "$FIREWALL_REFRESH_INTERVAL_SECONDS" "DUNE_FIREWALL_REFRESH_INTERVAL_SECONDS"
  validate_positive_int "$FIREWALL_REFRESH_ATTEMPTS" "DUNE_FIREWALL_REFRESH_ATTEMPTS"
  validate_nonnegative_int "$FIREWALL_REFRESH_RETRY_DELAY_SECONDS" "DUNE_FIREWALL_REFRESH_RETRY_DELAY_SECONDS"
}

refresh_domain_allowlist_entries() {
  local domain="$1"
  local requirement="$2"
  local reason="$3"
  local cidr_bits="$4"
  local attempts="$5"
  local delay_seconds="$6"
  local ips=""
  local added=0

  ips="$(resolve_ipv4s_with_retry "$domain" "$attempts" "$delay_seconds" || true)"
  if [ -z "$ips" ]; then
    if [ "$requirement" = "required" ]; then
      log_warn "Refresh failed to resolve required domain ${domain} (${reason})"
    else
      log_debug "Refresh skipped optional domain ${domain} (${reason}) due to DNS miss"
    fi
    return 1
  fi

  while IFS= read -r ip; do
    local network

    [ -z "$ip" ] && continue
    if [[ ! "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
      log_warn "Refresh skipped invalid IPv4 for ${domain}: ${ip}"
      continue
    fi

    if ! network="$(ipv4_to_cidr_network "$ip" "$cidr_bits")"; then
      log_warn "Refresh skipped unsupported CIDR bits for ${domain}: ${cidr_bits}"
      continue
    fi

    if ipset add --exist allowed-domains "$network"; then
      added=$((added + 1))
      log_debug "Refresh allow ${domain}: ${network}"
    fi
  done <<<"$ips"

  if [ "$added" -eq 0 ]; then
    return 1
  fi

  return 0
}

run_refresh_loop() {
  validate_refresh_config

  if [ "$FIREWALL_REFRESH_INTERVAL_SECONDS" -eq 0 ]; then
    log_info "Allowlist refresh loop disabled (DUNE_FIREWALL_REFRESH_INTERVAL_SECONDS=0)"
    exit 0
  fi

  if [ ! -s "$FIREWALL_REFRESH_DOMAINS_FILE" ]; then
    log_warn "Refresh loop has no domain file at ${FIREWALL_REFRESH_DOMAINS_FILE}; exiting"
    exit 0
  fi

  if ! ipset list allowed-domains >/dev/null 2>&1; then
    log_error "Refresh loop cannot find ipset 'allowed-domains'"
    exit 1
  fi

  mkdir -p "$FIREWALL_RUNTIME_DIR"
  printf '%s\n' "$$" > "$FIREWALL_REFRESH_PID_FILE"
  trap 'rm -f "$FIREWALL_REFRESH_PID_FILE"' EXIT

  log_info "Starting allowlist refresh loop: interval=${FIREWALL_REFRESH_INTERVAL_SECONDS}s attempts=${FIREWALL_REFRESH_ATTEMPTS} retry_delay=${FIREWALL_REFRESH_RETRY_DELAY_SECONDS}s"

  while true; do
    refreshed_domains=0
    failed_required_domains=0

    while IFS='|' read -r domain requirement reason cidr_bits _; do
      [ -z "${domain:-}" ] && continue
      if refresh_domain_allowlist_entries "$domain" "$requirement" "$reason" "${cidr_bits:-32}" "$FIREWALL_REFRESH_ATTEMPTS" "$FIREWALL_REFRESH_RETRY_DELAY_SECONDS"; then
        refreshed_domains=$((refreshed_domains + 1))
      else
        if [ "$requirement" = "required" ]; then
          failed_required_domains=$((failed_required_domains + 1))
        fi
      fi
    done < "$FIREWALL_REFRESH_DOMAINS_FILE"

    log_debug "Refresh cycle complete: refreshed_domains=${refreshed_domains} failed_required_domains=${failed_required_domains}"
    sleep "$FIREWALL_REFRESH_INTERVAL_SECONDS"
  done
}

stop_existing_refresh_loop() {
  local pid=""
  local state=""

  if [ ! -f "$FIREWALL_REFRESH_PID_FILE" ]; then
    return 0
  fi

  pid="$(cat "$FIREWALL_REFRESH_PID_FILE" 2>/dev/null || true)"
  if [ -z "$pid" ]; then
    rm -f "$FIREWALL_REFRESH_PID_FILE"
    return 0
  fi

  if kill -0 "$pid" >/dev/null 2>&1; then
    log_info "Stopping previous allowlist refresh loop (pid=${pid})"
    kill "$pid" >/dev/null 2>&1 || true
    for _ in $(seq 1 30); do
      state="$(ps -o stat= -p "$pid" 2>/dev/null | tr -d ' ')"
      if [ -z "$state" ] || [[ "$state" == Z* ]]; then
        break
      fi
      sleep 0.1
    done
    state="$(ps -o stat= -p "$pid" 2>/dev/null | tr -d ' ')"
    if [ -n "$state" ] && [[ ! "$state" == Z* ]]; then
      log_warn "Force stopping refresh loop (pid=${pid})"
      kill -9 "$pid" >/dev/null 2>&1 || true
    fi
  fi

  rm -f "$FIREWALL_REFRESH_PID_FILE"
}

start_refresh_loop() {
  local -n specs_ref=$1
  local pid=""

  validate_refresh_config
  mkdir -p "$FIREWALL_RUNTIME_DIR"

  stop_existing_refresh_loop

  : > "$FIREWALL_REFRESH_DOMAINS_FILE"
  for spec in "${specs_ref[@]}"; do
    printf '%s\n' "$spec" >> "$FIREWALL_REFRESH_DOMAINS_FILE"
  done

  if [ "$FIREWALL_REFRESH_INTERVAL_SECONDS" -eq 0 ]; then
    log_info "Allowlist refresh loop disabled (DUNE_FIREWALL_REFRESH_INTERVAL_SECONDS=0)"
    return 0
  fi

  if [ "${#specs_ref[@]}" -eq 0 ]; then
    log_warn "No refresh domains configured; skipping refresh loop start"
    return 0
  fi

  nohup /usr/local/bin/init-firewall.sh --refresh-loop >>"$FIREWALL_REFRESH_LOG_FILE" 2>&1 &
  pid="$!"
  printf '%s\n' "$pid" > "$FIREWALL_REFRESH_PID_FILE"
  log_info "Started allowlist refresh loop (pid=${pid}, interval=${FIREWALL_REFRESH_INTERVAL_SECONDS}s, domains=${#specs_ref[@]})"
}

allow_domain() {
  local domain="$1"
  local requirement="$2"
  local reason="$3"
  local cidr_bits="${4:-32}"
  local ips=""
  local entries_added_for_domain=0

  log_debug "Resolving ${domain} (${reason})"
  ips="$(resolve_ipv4s_with_retry "$domain" 5 1 || true)"

  if [ -z "$ips" ]; then
    if [ "$requirement" = "required" ]; then
      log_error "Failed to resolve required domain ${domain} (${reason})"
      exit 1
    else
      DOMAIN_OPTIONAL_SKIPPED_COUNT=$((DOMAIN_OPTIONAL_SKIPPED_COUNT + 1))
      log_warn "Failed to resolve optional domain ${domain} (${reason}), skipping"
      return 0
    fi
  fi

  DOMAIN_RESOLVED_COUNT=$((DOMAIN_RESOLVED_COUNT + 1))

  while read -r ip; do
    local network

    [ -z "$ip" ] && continue
    if [[ ! "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
      log_warn "Invalid IP from DNS for ${domain}: ${ip}, skipping"
      continue
    fi

    if ! network="$(ipv4_to_cidr_network "$ip" "$cidr_bits")"; then
      log_error "Unsupported CIDR bits for ${domain}: ${cidr_bits}"
      exit 1
    fi

    ipset add --exist allowed-domains "$network"
    log_debug "Allow ${domain}: ${network}"

    ALLOWLIST_ENTRY_ADDS=$((ALLOWLIST_ENTRY_ADDS + 1))
    DOMAIN_ALLOWLIST_ENTRY_ADDS=$((DOMAIN_ALLOWLIST_ENTRY_ADDS + 1))
    entries_added_for_domain=$((entries_added_for_domain + 1))
  done < <(echo "$ips")

  log_debug "Resolved ${domain}: entries_added=${entries_added_for_domain}"
}

DOMAIN_SPECS=()
REFRESH_DOMAIN_SPECS=()

load_domain_specs_from_config() {
  local domain=""
  local allow_requirement=""
  local refresh_requirement=""
  local cidr_bits=""
  local reason=""

  if [ ! -f "$FIREWALL_DOMAIN_CONFIG" ]; then
    log_error "Firewall domain config not found at ${FIREWALL_DOMAIN_CONFIG}"
    exit 1
  fi

  log_info "Loading firewall domain config from ${FIREWALL_DOMAIN_CONFIG}"

  while IFS=$'\t' read -r domain allow_requirement refresh_requirement cidr_bits reason; do
    [ -z "${domain:-}" ] && continue
    case "$domain" in
      \#*)
        continue
        ;;
      domain)
        continue
        ;;
    esac

    if [ -z "${reason:-}" ]; then
      log_error "Invalid firewall domain row for ${domain}: missing reason"
      exit 1
    fi

    case "$allow_requirement" in
      required|optional)
        ;;
      *)
        log_error "Invalid allow requirement for ${domain}: ${allow_requirement}"
        exit 1
        ;;
    esac

    case "$refresh_requirement" in
      required|optional|-)
        ;;
      *)
        log_error "Invalid refresh requirement for ${domain}: ${refresh_requirement}"
        exit 1
        ;;
    esac

    case "$cidr_bits" in
      16|24|32)
        ;;
      *)
        log_error "Invalid CIDR bits for ${domain}: ${cidr_bits}"
        exit 1
        ;;
    esac

    DOMAIN_SPECS+=("${domain}|${allow_requirement}|${reason}|${cidr_bits}")
    if [ "$refresh_requirement" != "-" ]; then
      REFRESH_DOMAIN_SPECS+=("${domain}|${refresh_requirement}|${reason}|${cidr_bits}")
    fi
  done < "$FIREWALL_DOMAIN_CONFIG"
}

verify_url_reachable() {
  local url="$1"
  if ! curl --connect-timeout 5 "$url" >/dev/null 2>&1; then
    log_error "Firewall verification failed - unable to reach $url"
    exit 1
  fi
  log_info "Firewall verification passed for $url"
}

main() {
  # Configure a strict egress firewall for the devcontainer while preserving
  # DNS, localhost, host-network connectivity, and a curated allowlist.
  local DOCKER_DNS_RULES=""
  local HOST_IP=""
  local HOST_NETWORK=""
  local gh_ranges=""
  local gh_cidrs=""
  local gh_cidrs_aggregated=""
  local raw_gh_count=0
  local agg_gh_count=0

  ALLOWLIST_ENTRY_ADDS=0
  GH_ALLOWLIST_ENTRY_ADDS=0
  DOMAIN_ALLOWLIST_ENTRY_ADDS=0
  DOMAIN_RESOLVED_COUNT=0
  DOMAIN_OPTIONAL_SKIPPED_COUNT=0
  DOMAIN_SPECS=()
  REFRESH_DOMAIN_SPECS=()

  validate_refresh_config
  load_domain_specs_from_config
  stop_existing_refresh_loop

  # Re-initialization can happen when previous policies are already DROP.
  # Temporarily open policies while rebuilding rules and allowlists.
  iptables -P INPUT ACCEPT
  iptables -P FORWARD ACCEPT
  iptables -P OUTPUT ACCEPT

  # Preserve Docker's internal DNS NAT rules before flushing tables.
  DOCKER_DNS_RULES="$(iptables-save -t nat | grep "127\.0\.0\.11" || true)"

  # Reset any existing rules/chains and previous allowlist set.
  iptables -F
  iptables -X
  iptables -t nat -F
  iptables -t nat -X
  iptables -t mangle -F
  iptables -t mangle -X
  ipset destroy allowed-domains 2>/dev/null || true

  # Restore Docker-managed DNS NAT rules so container name resolution still works.
  if [ -n "$DOCKER_DNS_RULES" ]; then
    log_info "Restoring Docker DNS rules"
    iptables -t nat -N DOCKER_OUTPUT 2>/dev/null || true
    iptables -t nat -N DOCKER_POSTROUTING 2>/dev/null || true
    echo "$DOCKER_DNS_RULES" | xargs -L 1 iptables -t nat
  else
    log_info "No Docker DNS rules found to restore"
  fi

  # Baseline allowances needed before default DROP policies are applied.
  iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
  iptables -A INPUT -p udp --sport 53 -j ACCEPT
  iptables -A OUTPUT -p tcp --dport 22 -j ACCEPT
  iptables -A INPUT -p tcp --sport 22 -m state --state ESTABLISHED -j ACCEPT
  iptables -A INPUT -i lo -j ACCEPT
  iptables -A OUTPUT -o lo -j ACCEPT

  # All allowed remote destinations are tracked in this ipset.
  ipset create allowed-domains hash:net

  # Allow all published GitHub networks from GitHub metadata. This covers
  # GitHub API/web/git/release assets used by gh and many tool installers.
  log_info "Fetching GitHub IP ranges"
  gh_ranges="$(curl -fsSL https://api.github.com/meta)"
  if [ -z "$gh_ranges" ]; then
    log_error "Failed to fetch GitHub IP ranges"
    exit 1
  fi

  if ! echo "$gh_ranges" | jq -e '.web and .api and .git' >/dev/null; then
    log_error "GitHub API response missing required fields"
    exit 1
  fi

  gh_cidrs="$(echo "$gh_ranges" | jq -r '.. | strings | select(test("^[0-9]{1,3}(\\.[0-9]{1,3}){3}/[0-9]{1,2}$"))')"
  if [ -z "$gh_cidrs" ]; then
    log_error "No CIDR ranges found in GitHub meta response"
    exit 1
  fi

  gh_cidrs_aggregated="$(printf '%s\n' "$gh_cidrs" | sort -u | aggregate -q)"
  raw_gh_count="$(printf '%s\n' "$gh_cidrs" | sed '/^$/d' | wc -l | tr -d ' ')"
  agg_gh_count="$(printf '%s\n' "$gh_cidrs_aggregated" | sed '/^$/d' | wc -l | tr -d ' ')"

  while read -r cidr; do
    [ -z "$cidr" ] && continue
    if [[ ! "$cidr" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}/[0-9]{1,2}$ ]]; then
      log_error "Invalid CIDR range from GitHub meta: $cidr"
      exit 1
    fi
    ipset add --exist allowed-domains "$cidr"
    ALLOWLIST_ENTRY_ADDS=$((ALLOWLIST_ENTRY_ADDS + 1))
    GH_ALLOWLIST_ENTRY_ADDS=$((GH_ALLOWLIST_ENTRY_ADDS + 1))
    log_debug "GitHub allow CIDR: $cidr"
  done < <(printf '%s\n' "$gh_cidrs_aggregated")

  log_info "GitHub CIDRs loaded: raw=${raw_gh_count} aggregated=${agg_gh_count}"

  log_info "Resolving allowlist domains (${#DOMAIN_SPECS[@]} specs)"
  for spec in "${DOMAIN_SPECS[@]}"; do
    IFS='|' read -r domain requirement reason cidr_bits <<<"$spec"
    allow_domain "$domain" "$requirement" "$reason" "${cidr_bits:-32}"
  done

  # Allow host-network communication (docker bridge gateway).
  HOST_IP="$(ip route | grep default | cut -d" " -f3)"
  if [ -z "$HOST_IP" ]; then
    log_error "Failed to detect host IP"
    exit 1
  fi

  HOST_NETWORK="$(echo "$HOST_IP" | sed "s/\.[0-9]*$/.0\/24/")"
  log_info "Host network detected: $HOST_NETWORK"
  iptables -A INPUT -s "$HOST_NETWORK" -j ACCEPT
  iptables -A OUTPUT -d "$HOST_NETWORK" -j ACCEPT

  # Apply default deny and then permit established + allowlisted outbound.
  iptables -P INPUT DROP
  iptables -P FORWARD DROP
  iptables -P OUTPUT DROP
  iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
  iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
  iptables -A OUTPUT -m set --match-set allowed-domains dst -j ACCEPT
  iptables -A OUTPUT -j REJECT --reject-with icmp-admin-prohibited

  log_info "Firewall rules applied: allowlist_entries=${ALLOWLIST_ENTRY_ADDS} github_entries=${GH_ALLOWLIST_ENTRY_ADDS} domain_entries=${DOMAIN_ALLOWLIST_ENTRY_ADDS} domains_resolved=${DOMAIN_RESOLVED_COUNT} optional_domains_skipped=${DOMAIN_OPTIONAL_SKIPPED_COUNT}"

  start_refresh_loop REFRESH_DOMAIN_SPECS

  # Ensure deny-by-default still works.
  if curl --connect-timeout 5 https://example.com >/dev/null 2>&1; then
    log_error "Firewall verification failed - was able to reach https://example.com"
    exit 1
  else
    log_info "Firewall verification passed - unable to reach https://example.com"
  fi

  # Verify core endpoints that should always be reachable for this image.
  verify_url_reachable "https://api.github.com/zen"
  verify_url_reachable "https://chatgpt.com/"
  verify_url_reachable "https://api.openai.com/"
  verify_url_reachable "https://opencode.ai/zen"
  verify_url_reachable "https://api.z.ai/"
  verify_url_reachable "https://mcp.grep.app/"
  verify_url_reachable "https://oauth2.googleapis.com/token"

  log_info "Firewall initialization complete"
}

if [ "${1:-}" = "--refresh-loop" ]; then
  run_refresh_loop
  exit 0
fi

main "$@"
