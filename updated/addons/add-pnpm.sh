#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-pnpm] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
PNPM_VERSION="${SAND_PNPM_VERSION:-latest}"
NPM_PREFIX="${NPM_CONFIG_PREFIX:-/usr/local/share/npm-global}"
NPM_GLOBAL_BIN="${NPM_PREFIX}/bin"

log() {
  echo "[add-pnpm] $*"
}

run_as_target_user() {
  runuser -u "$TARGET_USER" -- env \
    HOME="$TARGET_HOME" \
    USER="$TARGET_USER" \
    LOGNAME="$TARGET_USER" \
    NPM_CONFIG_PREFIX="$NPM_PREFIX" \
    PATH="${NPM_GLOBAL_BIN}:$PATH" \
    "$@"
}

log "Installing pnpm@${PNPM_VERSION} globally"
run_as_target_user npm install -g "pnpm@${PNPM_VERSION}"

log "Done. Verify with 'pnpm --version'."
