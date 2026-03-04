#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-playwright] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
NPM_PREFIX="${NPM_CONFIG_PREFIX:-/usr/local/share/npm-global}"
NPM_GLOBAL_BIN="${NPM_PREFIX}/bin"
PLAYWRIGHT_BROWSERS="${SAND_PLAYWRIGHT_BROWSERS:-chromium firefox webkit}"
PLAYWRIGHT_INSTALL_ATTEMPTS="${SAND_PLAYWRIGHT_INSTALL_ATTEMPTS:-5}"
PLAYWRIGHT_RETRY_DELAY_SECONDS="${SAND_PLAYWRIGHT_RETRY_DELAY_SECONDS:-5}"
PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT="${SAND_PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT:-120000}"

log() {
  echo "[add-playwright] $*"
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
    [ -z "$ip" ] && continue

    if [ "$cidr_bits" = "32" ]; then
      ipset add --exist allowed-domains "$ip"
    else
      local cidr
      case "$cidr_bits" in
        16)
          cidr="$(echo "$ip" | awk -F. '{print $1 "." $2 ".0.0/16"}')"
          ;;
        24)
          cidr="$(echo "$ip" | awk -F. '{print $1 "." $2 "." $3 ".0/24"}')"
          ;;
        *)
          echo "[add-playwright] Unsupported CIDR bits for firewall refresh: $cidr_bits" >&2
          return 1
          ;;
      esac
      ipset add --exist allowed-domains "$cidr"
    fi

    added=$((added + 1))
  done <<<"$ips"

  if [ "$added" -eq 0 ]; then
    return 1
  fi

  return 0
}

refresh_playwright_firewall_allowlist() {
  local refreshed=0

  if ! command -v ipset >/dev/null 2>&1; then
    return 0
  fi

  if ! ipset list allowed-domains >/dev/null 2>&1; then
    return 0
  fi

  log "Refreshing firewall allowlist for Playwright browser download hosts"

  if add_ipset_allow_entries_for_domain "cdn.playwright.dev" "16"; then
    refreshed=$((refreshed + 1))
  fi
  if add_ipset_allow_entries_for_domain "playwright.download.prss.microsoft.com" "16"; then
    refreshed=$((refreshed + 1))
  fi
  if add_ipset_allow_entries_for_domain "storage.googleapis.com" "16"; then
    refreshed=$((refreshed + 1))
  fi

  if [ "$refreshed" -eq 0 ]; then
    echo "[add-playwright] Unable to refresh firewall allowlist for Playwright download hosts" >&2
    exit 1
  fi
}

run_as_target_user() {
  runuser -u "$TARGET_USER" -- env \
    HOME="$TARGET_HOME" \
    USER="$TARGET_USER" \
    LOGNAME="$TARGET_USER" \
    XDG_CACHE_HOME="${TARGET_HOME}/.cache" \
    XDG_CONFIG_HOME="${TARGET_HOME}/.config" \
    NPM_CONFIG_PREFIX="$NPM_PREFIX" \
    PATH="${NPM_GLOBAL_BIN}:$PATH" \
    "$@"
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
  echo "[add-playwright] Invalid SAND_PLAYWRIGHT_INSTALL_ATTEMPTS='${PLAYWRIGHT_INSTALL_ATTEMPTS}'" >&2
  exit 1
fi

if [[ ! "$PLAYWRIGHT_RETRY_DELAY_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "[add-playwright] Invalid SAND_PLAYWRIGHT_RETRY_DELAY_SECONDS='${PLAYWRIGHT_RETRY_DELAY_SECONDS}'" >&2
  exit 1
fi

if [[ ! "$PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT" =~ ^[1-9][0-9]*$ ]]; then
  echo "[add-playwright] Invalid SAND_PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT='${PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT}'" >&2
  exit 1
fi

read -r -a PLAYWRIGHT_BROWSER_LIST <<<"$PLAYWRIGHT_BROWSERS"
if [ "${#PLAYWRIGHT_BROWSER_LIST[@]}" -eq 0 ]; then
  echo "[add-playwright] No browsers specified via SAND_PLAYWRIGHT_BROWSERS" >&2
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
