#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-playwright] must run as root" >&2
  exit 1
fi

TARGET_USER="${DUNE_TARGET_USER:-node}"
TARGET_HOME="${DUNE_TARGET_HOME:-/home/${TARGET_USER}}"
NPM_PREFIX="${NPM_CONFIG_PREFIX:-/usr/local/share/npm-global}"
NPM_GLOBAL_BIN="${NPM_PREFIX}/bin"
PLAYWRIGHT_BROWSERS="${DUNE_PLAYWRIGHT_BROWSERS:-chromium firefox webkit}"
PLAYWRIGHT_INSTALL_ATTEMPTS="${DUNE_PLAYWRIGHT_INSTALL_ATTEMPTS:-5}"
PLAYWRIGHT_RETRY_DELAY_SECONDS="${DUNE_PLAYWRIGHT_RETRY_DELAY_SECONDS:-5}"
PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT="${DUNE_PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT:-120000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
FIREWALL_DOMAIN_CONFIG="${DUNE_FIREWALL_DOMAIN_CONFIG:-/usr/local/lib/dune/runtime/firewall-domains.tsv}"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi

if [ ! -f "$FIREWALL_DOMAIN_CONFIG" ]; then
  FIREWALL_DOMAIN_CONFIG="${SCRIPT_DIR}/../runtime/firewall-domains.tsv"
fi

. "$UTILS_PATH"

log() {
  echo "[add-playwright] $*"
}

add_ipset_allow_entries_for_domain() {
  local domain="$1"
  local cidr_bits="$2"
  local ips=""
  local added=0

  ips="$(resolve_ipv4s_with_retry "$domain" 5 1 || true)"
  if [ -z "$ips" ]; then
    log "WARNING: unable to resolve ${domain} while refreshing firewall allowlist"
    return 1
  fi

  while IFS= read -r ip; do
    local network

    [ -z "$ip" ] && continue
    if ! network="$(ipv4_to_cidr_network "$ip" "$cidr_bits")"; then
      echo "[add-playwright] Unsupported CIDR bits for firewall refresh: $cidr_bits" >&2
      return 1
    fi
    ipset add --exist allowed-domains "$network"

    added=$((added + 1))
  done <<<"$ips"

  if [ "$added" -eq 0 ]; then
    return 1
  fi

  return 0
}

load_playwright_firewall_specs() {
  local domain=""
  local _allow_requirement=""
  local _refresh_requirement=""
  local cidr_bits=""
  local reason=""
  local -a specs=()

  if [ ! -f "$FIREWALL_DOMAIN_CONFIG" ]; then
    echo "[add-playwright] Firewall domain config not found: ${FIREWALL_DOMAIN_CONFIG}" >&2
    exit 1
  fi

  while IFS=$'\t' read -r domain _allow_requirement _refresh_requirement cidr_bits reason; do
    [ -z "${domain:-}" ] && continue
    [ "$domain" = "domain" ] && continue
    [[ "$domain" == \#* ]] && continue
    if [[ "$reason" == Playwright\ * ]]; then
      specs+=("${domain}|${cidr_bits}")
    fi
  done < "$FIREWALL_DOMAIN_CONFIG"

  if [ "${#specs[@]}" -eq 0 ]; then
    echo "[add-playwright] No Playwright firewall domains found in ${FIREWALL_DOMAIN_CONFIG}" >&2
    exit 1
  fi

  printf '%s\n' "${specs[@]}"
}

refresh_playwright_firewall_allowlist() {
  local refreshed=0
  local spec=""
  local domain=""
  local cidr_bits=""

  if ! command -v ipset >/dev/null 2>&1; then
    return 0
  fi

  if ! ipset list allowed-domains >/dev/null 2>&1; then
    return 0
  fi

  log "Refreshing firewall allowlist for Playwright browser download hosts"

  while IFS= read -r spec; do
    [ -z "$spec" ] && continue
    IFS='|' read -r domain cidr_bits <<<"$spec"
    if add_ipset_allow_entries_for_domain "$domain" "$cidr_bits"; then
      refreshed=$((refreshed + 1))
    fi
  done < <(load_playwright_firewall_specs)

  if [ "$refreshed" -eq 0 ]; then
    echo "[add-playwright] Unable to refresh firewall allowlist for Playwright download hosts" >&2
    exit 1
  fi
}

run_with_retry() {
  local attempts="$1"
  local delay_seconds="$2"
  local description="$3"
  shift 3

  local attempt=1
  local rc=0

  while true; do
    if "$@"; then
      return 0
    fi

    rc=$?
    if [ "$attempt" -ge "$attempts" ]; then
      echo "[add-playwright] ${description} failed after ${attempts} attempts" >&2
      return "$rc"
    fi

    log "Attempt ${attempt}/${attempts} failed for ${description}; retrying in ${delay_seconds}s"
    attempt=$((attempt + 1))
    sleep "$delay_seconds"
  done
}

if [[ ! "$PLAYWRIGHT_INSTALL_ATTEMPTS" =~ ^[1-9][0-9]*$ ]]; then
  echo "[add-playwright] Invalid DUNE_PLAYWRIGHT_INSTALL_ATTEMPTS='${PLAYWRIGHT_INSTALL_ATTEMPTS}'" >&2
  exit 1
fi

if [[ ! "$PLAYWRIGHT_RETRY_DELAY_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "[add-playwright] Invalid DUNE_PLAYWRIGHT_RETRY_DELAY_SECONDS='${PLAYWRIGHT_RETRY_DELAY_SECONDS}'" >&2
  exit 1
fi

if [[ ! "$PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT" =~ ^[1-9][0-9]*$ ]]; then
  echo "[add-playwright] Invalid DUNE_PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT='${PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT}'" >&2
  exit 1
fi

read -r -a PLAYWRIGHT_BROWSER_LIST <<<"$PLAYWRIGHT_BROWSERS"
if [ "${#PLAYWRIGHT_BROWSER_LIST[@]}" -eq 0 ]; then
  echo "[add-playwright] No browsers specified via DUNE_PLAYWRIGHT_BROWSERS" >&2
  exit 1
fi

log "Ensuring target user cache/config directories exist"
mkdir -p "${TARGET_HOME}/.cache" "${TARGET_HOME}/.config"
chown -R "${TARGET_USER}:${TARGET_USER}" "${TARGET_HOME}/.cache" "${TARGET_HOME}/.config"

log "Installing Playwright CLI globally"
run_as_target_user npm install -g playwright@latest

PLAYWRIGHT_BIN="${NPM_PREFIX}/bin/playwright"
if [ ! -x "$PLAYWRIGHT_BIN" ]; then
  PLAYWRIGHT_BIN="$(command -v playwright || true)"
fi

if [ -z "$PLAYWRIGHT_BIN" ] || [ ! -x "$PLAYWRIGHT_BIN" ]; then
  echo "[add-playwright] Unable to locate playwright binary after install" >&2
  exit 1
fi

refresh_playwright_firewall_allowlist

log "Installing Playwright system dependencies (${PLAYWRIGHT_BROWSER_LIST[*]})"
run_with_retry 3 5 "playwright install-deps ${PLAYWRIGHT_BROWSER_LIST[*]}" \
  "$PLAYWRIGHT_BIN" install-deps "${PLAYWRIGHT_BROWSER_LIST[@]}"

log "Installing Playwright browser binaries for ${TARGET_USER} (${PLAYWRIGHT_BROWSER_LIST[*]})"
for browser in "${PLAYWRIGHT_BROWSER_LIST[@]}"; do
  run_with_retry "$PLAYWRIGHT_INSTALL_ATTEMPTS" "$PLAYWRIGHT_RETRY_DELAY_SECONDS" "playwright install ${browser}" \
    run_as_target_user env \
      PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT="$PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT" \
      "$PLAYWRIGHT_BIN" install "$browser"
done

log "Done. Verify with 'playwright --version' and run your e2e suite."
