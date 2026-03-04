#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-playwright] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
NPM_PREFIX="${NPM_CONFIG_PREFIX:-/usr/local/share/npm-global}"

log() {
  echo "[add-playwright] $*"
}

run_as_target_user() {
  runuser -u "$TARGET_USER" -- env \
    HOME="$TARGET_HOME" \
    USER="$TARGET_USER" \
    LOGNAME="$TARGET_USER" \
    XDG_CACHE_HOME="${TARGET_HOME}/.cache" \
    XDG_CONFIG_HOME="${TARGET_HOME}/.config" \
    PATH="$PATH" \
    "$@"
}

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

log "Installing Playwright system dependencies"
"$PLAYWRIGHT_BIN" install-deps chromium firefox webkit

log "Installing Playwright browser binaries for ${TARGET_USER}"
run_as_target_user "$PLAYWRIGHT_BIN" install chromium firefox webkit

log "Done. Verify with 'playwright --version' and run your e2e suite."
