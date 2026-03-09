#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

DEBUG_FIREWALL="${SAND_FIREWALL_DEBUG:-0}"
FIREWALL_DEBUG_ENABLED=0
case "$(printf '%s' "$DEBUG_FIREWALL" | tr '[:upper:]' '[:lower:]')" in
  1|true|yes|on)
    FIREWALL_DEBUG_ENABLED=1
    ;;
esac

FIREWALL_RUNTIME_DIR="/run/sand"
FIREWALL_REFRESH_PID_FILE="${FIREWALL_RUNTIME_DIR}/firewall-refresh.pid"
FIREWALL_REFRESH_DOMAINS_FILE="${FIREWALL_RUNTIME_DIR}/firewall-refresh-domains.tsv"
FIREWALL_REFRESH_LOG_FILE="${FIREWALL_RUNTIME_DIR}/firewall-refresh.log"
FIREWALL_REFRESH_INTERVAL_SECONDS="${SAND_FIREWALL_REFRESH_INTERVAL_SECONDS:-10}"
FIREWALL_REFRESH_ATTEMPTS="${SAND_FIREWALL_REFRESH_ATTEMPTS:-3}"
FIREWALL_REFRESH_RETRY_DELAY_SECONDS="${SAND_FIREWALL_REFRESH_RETRY_DELAY_SECONDS:-1}"

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
  validate_nonnegative_int "$FIREWALL_REFRESH_INTERVAL_SECONDS" "SAND_FIREWALL_REFRESH_INTERVAL_SECONDS"
  validate_positive_int "$FIREWALL_REFRESH_ATTEMPTS" "SAND_FIREWALL_REFRESH_ATTEMPTS"
  validate_nonnegative_int "$FIREWALL_REFRESH_RETRY_DELAY_SECONDS" "SAND_FIREWALL_REFRESH_RETRY_DELAY_SECONDS"
}

resolve_ipv4s_with_retry() {
  local domain="$1"
  local attempts="${2:-5}"
  local delay_seconds="${3:-1}"
  local dig_ips=""
  local getent_ips=""
  local ips=""

  for _ in $(seq 1 "$attempts"); do
    dig_ips="$(dig +short A "$domain" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' || true)"
    getent_ips="$(getent ahostsv4 "$domain" 2>/dev/null | awk '{print $1}' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' || true)"
    ips="$(printf '%s\n%s\n' "$dig_ips" "$getent_ips" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -u || true)"
    if [ -n "$ips" ]; then
      printf '%s\n' "$ips"
      return 0
    fi
    sleep "$delay_seconds"
  done

  return 1
}

refresh_domain_allowlist_entries() {
  local domain="$1"
  local requirement="$2"
  local reason="$3"
  local attempts="$4"
  local delay_seconds="$5"
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
    [ -z "$ip" ] && continue
    if [[ ! "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
      log_warn "Refresh skipped invalid IPv4 for ${domain}: ${ip}"
      continue
    fi

    if ipset add --exist allowed-domains "$ip"; then
      added=$((added + 1))
      log_debug "Refresh allow ${domain}: ${ip}/32"
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
    log_info "Allowlist refresh loop disabled (SAND_FIREWALL_REFRESH_INTERVAL_SECONDS=0)"
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

    while IFS='|' read -r domain requirement reason _; do
      [ -z "${domain:-}" ] && continue
      if refresh_domain_allowlist_entries "$domain" "$requirement" "$reason" "$FIREWALL_REFRESH_ATTEMPTS" "$FIREWALL_REFRESH_RETRY_DELAY_SECONDS"; then
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
    log_info "Allowlist refresh loop disabled (SAND_FIREWALL_REFRESH_INTERVAL_SECONDS=0)"
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
    [ -z "$ip" ] && continue
    if [[ ! "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
      log_warn "Invalid IP from DNS for ${domain}: ${ip}, skipping"
      continue
    fi

    if [ "$cidr_bits" = "32" ]; then
      ipset add --exist allowed-domains "$ip"
      log_debug "Allow ${domain}: ${ip}/32"
    else
      local cidr
      cidr="$(echo "$ip" | awk -F. -v bits="$cidr_bits" '{print $1 "." $2 "." $3 ".0/" bits}')"
      ipset add --exist allowed-domains "$cidr"
      log_debug "Allow ${domain}: ${cidr}"
    fi

    ALLOWLIST_ENTRY_ADDS=$((ALLOWLIST_ENTRY_ADDS + 1))
    DOMAIN_ALLOWLIST_ENTRY_ADDS=$((DOMAIN_ALLOWLIST_ENTRY_ADDS + 1))
    entries_added_for_domain=$((entries_added_for_domain + 1))
  done < <(echo "$ips")

  log_debug "Resolved ${domain}: entries_added=${entries_added_for_domain}"
}

DOMAIN_SPECS=()
REFRESH_DOMAIN_SPECS=()
add_required_domain() {
  DOMAIN_SPECS+=("$1|required|$2")
}
add_optional_domain() {
  DOMAIN_SPECS+=("$1|optional|$2")
}
add_required_domain_with_cidr() {
  DOMAIN_SPECS+=("$1|required|$2|$3")
}
add_optional_domain_with_cidr() {
  DOMAIN_SPECS+=("$1|optional|$2|$3")
}
add_refresh_domain() {
  REFRESH_DOMAIN_SPECS+=("$1|$2|$3")
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

  # GitHub CLI tooling.
  add_optional_domain "cli.github.com" "gh CLI package metadata/download host"

  # Core package registries used by npm/pnpm/yarn and JS tooling.
  add_required_domain "registry.npmjs.org" "npm registry for npm/pnpm/corepack package manager assets"
  add_optional_domain "npm.jsr.io" "JSR registry support used by modern pnpm/npm workflows"
  add_optional_domain "repo.yarnpkg.com" "Yarn modern releases (yarn set version / corepack flows)"

  # Claude Code + telemetry endpoints.
  add_required_domain "api.anthropic.com" "Claude Code API"
  add_optional_domain "sentry.io" "error reporting used by some CLIs"
  add_optional_domain "statsig.anthropic.com" "Claude feature flag/telemetry endpoint"
  add_optional_domain "statsig.com" "Claude feature flag/telemetry backend"

  # VS Code/devcontainer extension update endpoints.
  add_optional_domain "marketplace.visualstudio.com" "VS Code extension marketplace"
  add_optional_domain "vscode.blob.core.windows.net" "VS Code extension asset blobs"
  add_optional_domain "update.code.visualstudio.com" "VS Code update checks"

  # OpenAI/Codex endpoints.
  add_required_domain "api.openai.com" "OpenAI API"
  add_required_domain "auth.openai.com" "OpenAI auth handoffs"
  add_required_domain "chatgpt.com" "Codex responses endpoint under chatgpt.com/backend-api/..."
  add_optional_domain "chat.openai.com" "legacy ChatGPT host used by some auth/browser flows"
  add_optional_domain "openai.com" "OpenAI web redirects and auth support"

  # OpenCode provider endpoints.
  add_required_domain_with_cidr "opencode.ai" "OpenCode Zen API/auth endpoint (allow /24 due edge IP churn)" "24"
  add_required_domain_with_cidr "api.z.ai" "Z.AI Coding Plan provider API endpoint (allow /24 due edge IP churn)" "24"
  add_optional_domain "z.ai" "Z.AI API console and account management"

  # Gemini and Google auth endpoints.
  add_required_domain "generativelanguage.googleapis.com" "Gemini API"
  add_required_domain "accounts.google.com" "Google account auth"
  add_required_domain "oauth2.googleapis.com" "OAuth token exchange for Gemini CLI"
  add_optional_domain "play.googleapis.com" "Google API backend dependencies"
  add_optional_domain "aiplatform.googleapis.com" "Vertex AI endpoint (if Gemini CLI uses Vertex mode)"
  add_optional_domain "cloudcode-pa.googleapis.com" "Google developer tooling backend"
  add_optional_domain "iamcredentials.googleapis.com" "GCP service-account token exchange"
  add_refresh_domain "generativelanguage.googleapis.com" "required" "Gemini API"
  add_refresh_domain "accounts.google.com" "required" "Google account auth"
  add_refresh_domain "oauth2.googleapis.com" "required" "OAuth token exchange for Gemini CLI"
  add_refresh_domain "play.googleapis.com" "optional" "Google API backend dependencies"
  add_refresh_domain "aiplatform.googleapis.com" "optional" "Vertex AI endpoint (if Gemini CLI uses Vertex mode)"
  add_refresh_domain "cloudcode-pa.googleapis.com" "optional" "Google developer tooling backend"
  add_refresh_domain "iamcredentials.googleapis.com" "optional" "GCP service-account token exchange"

  # MCP servers configured in this image.
  add_required_domain_with_cidr "mcp.grep.app" "grep MCP server (allow /24 due edge IP churn)" "24"
  add_required_domain_with_cidr "mcp.context7.com" "Context7 MCP server (allow /24 due edge IP churn)" "24"
  add_optional_domain "accounts.context7.com" "Context7 auth/session endpoints"
  add_required_domain_with_cidr "mcp.exa.ai" "Exa MCP server (allow /24 due edge IP churn)" "24"
  add_optional_domain "auth.exa.ai" "Exa auth endpoints"
  add_optional_domain "accounts.exa.ai" "Exa account endpoints"

  # Additional tool install scripts fetched by install scripts in container/setup.
  add_optional_domain "raw.githubusercontent.com" "beads/beads_viewer installer scripts"
  add_optional_domain "mise.run" "mise bootstrap install script"
  add_optional_domain "getmic.ro" "micro editor installer script"
  add_optional_domain "deb.debian.org" "Debian apt repositories for optional addon installs"
  add_optional_domain "security.debian.org" "Debian security apt repository for optional addon installs"
  add_optional_domain_with_cidr "cdn.playwright.dev" "Playwright browser download CDN (allow /16 due CDN edge IP churn)" "16"
  add_optional_domain_with_cidr "playwright.download.prss.microsoft.com" "Playwright browser download fallback CDN (allow /16 due CDN edge IP churn)" "16"
  add_optional_domain_with_cidr "storage.googleapis.com" "Playwright Chromium CFT redirected download host (allow /16 due Google edge IP churn)" "16"

  # mise-managed runtime/tool domains.
  add_optional_domain "nodejs.org" "mise node backend official Node.js binary downloads"
  add_optional_domain "unofficial-builds.nodejs.org" "optional Node unofficial builds via mise node.mirror_url"
  add_optional_domain "pypi.org" "uv/pip Python package index"
  add_optional_domain "files.pythonhosted.org" "Python package file downloads"
  add_optional_domain "astral.sh" "uv installer and release bootstrap endpoint"
  add_optional_domain "go.dev" "Go version metadata and release index"
  add_optional_domain "dl.google.com" "Go toolchain tarballs used by go/mise installs"
  add_optional_domain "proxy.golang.org" "Go module proxy used by go command"
  add_optional_domain "sum.golang.org" "Go checksum database used by go command"
  add_optional_domain "rustup.rs" "rustup installer landing host"
  add_optional_domain "sh.rustup.rs" "rustup shell installer script"
  add_optional_domain "static.rust-lang.org" "Rust toolchain/dist downloads"
  add_optional_domain "crates.io" "Rust crate registry API"
  add_optional_domain "index.crates.io" "Rust sparse index"
  add_optional_domain "static.crates.io" "Rust crate tarball downloads"

  # Optional toolchain addon domains.
  add_optional_domain "mise-versions.jdx.dev" "mise tool version metadata"

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
