#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-gemini] must run as root" >&2
  exit 1
fi

TARGET_USER="${DUNE_TARGET_USER:-node}"
TARGET_HOME="${DUNE_TARGET_HOME:-/home/${TARGET_USER}}"
GEMINI_VERSION="${DUNE_GEMINI_VERSION:-latest}"
NPM_PREFIX="${NPM_CONFIG_PREFIX:-/usr/local/share/npm-global}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi
. "$UTILS_PATH"

log() {
  echo "[add-gemini] $*"
}

mkdir -p "${TARGET_HOME}/.gemini"
chown -R "${TARGET_USER}:${TARGET_USER}" "${TARGET_HOME}/.gemini"

log "Installing @google/gemini-cli${GEMINI_VERSION:+@${GEMINI_VERSION}} globally"
install_npm_global_package @google/gemini-cli "$GEMINI_VERSION"

log "Done. Verify with 'gemini --version'."
