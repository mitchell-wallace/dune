#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-gemini] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
GEMINI_VERSION="${SAND_GEMINI_VERSION:-latest}"
NPM_PREFIX="${NPM_CONFIG_PREFIX:-/usr/local/share/npm-global}"
NPM_GLOBAL_BIN="${NPM_PREFIX}/bin"

log() {
  echo "[add-gemini] $*"
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

mkdir -p "${TARGET_HOME}/.gemini"
chown -R "${TARGET_USER}:${TARGET_USER}" "${TARGET_HOME}/.gemini"

if [ "$GEMINI_VERSION" = "latest" ]; then
  log "Installing @google/gemini-cli globally"
  run_as_target_user npm install -g @google/gemini-cli
else
  log "Installing @google/gemini-cli@${GEMINI_VERSION} globally"
  run_as_target_user npm install -g "@google/gemini-cli@${GEMINI_VERSION}"
fi

log "Done. Verify with 'gemini --version'."
