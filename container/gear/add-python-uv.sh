#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-python-uv] must run as root" >&2
  exit 1
fi

TARGET_USER="${DUNE_TARGET_USER:-node}"
TARGET_HOME="${DUNE_TARGET_HOME:-/home/${TARGET_USER}}"
PYTHON_VERSION="${DUNE_PYTHON_VERSION:-latest}"
UV_VERSION="${DUNE_UV_VERSION:-latest}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
DUNE_TARGET_EXTRA_PATH="${TARGET_HOME}/.local/bin:${TARGET_HOME}/.local/share/mise/shims"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi
. "$UTILS_PATH"

log() {
  echo "[add-python-uv] $*"
}

export DUNE_TARGET_EXTRA_PATH

ensure_mise_available || {
  echo "[add-python-uv] mise is required but not found for ${TARGET_USER}" >&2
  exit 1
}

log "Installing uv via mise (${UV_VERSION})"
install_mise_tool uv "$UV_VERSION"

log "Installing python via mise (${PYTHON_VERSION})"
install_mise_tool python "$PYTHON_VERSION"
run_as_target_user mise reshim

log "Done. Verify with 'uv --version' and 'python --version'."
