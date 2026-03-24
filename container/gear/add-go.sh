#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-go] must run as root" >&2
  exit 1
fi

TARGET_USER="${DUNE_TARGET_USER:-node}"
TARGET_HOME="${DUNE_TARGET_HOME:-/home/${TARGET_USER}}"
GO_VERSION="${DUNE_GO_VERSION:-latest}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
DUNE_TARGET_EXTRA_PATH="${TARGET_HOME}/.local/bin:${TARGET_HOME}/.local/share/mise/shims"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi
. "$UTILS_PATH"

log() {
  echo "[add-go] $*"
}

export DUNE_TARGET_EXTRA_PATH

ensure_mise_available || {
  echo "[add-go] mise is required but not found for ${TARGET_USER}" >&2
  exit 1
}

log "Installing go via mise (${GO_VERSION})"
install_mise_tool go "$GO_VERSION"
run_as_target_user mise reshim

log "Done. Verify with 'go version'."
