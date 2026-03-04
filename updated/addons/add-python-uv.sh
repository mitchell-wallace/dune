#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-python-uv] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
PYTHON_VERSION="${SAND_PYTHON_VERSION:-latest}"
UV_VERSION="${SAND_UV_VERSION:-latest}"

log() {
  echo "[add-python-uv] $*"
}

run_as_target_user() {
  runuser -u "$TARGET_USER" -- env \
    HOME="$TARGET_HOME" \
    USER="$TARGET_USER" \
    LOGNAME="$TARGET_USER" \
    PATH="${TARGET_HOME}/.local/bin:${TARGET_HOME}/.local/share/mise/shims:${PATH}" \
    "$@"
}

if ! run_as_target_user command -v mise >/dev/null 2>&1; then
  echo "[add-python-uv] mise is required but not found for ${TARGET_USER}" >&2
  exit 1
fi

log "Installing uv via mise (${UV_VERSION})"
if [ "$UV_VERSION" = "latest" ]; then
  run_as_target_user mise use -g uv@latest
else
  run_as_target_user mise use -g "uv@${UV_VERSION}"
fi

log "Installing python via mise (${PYTHON_VERSION})"
if [ "$PYTHON_VERSION" = "latest" ]; then
  run_as_target_user mise use -g python@latest
else
  run_as_target_user mise use -g "python@${PYTHON_VERSION}"
fi

run_as_target_user mise reshim

log "Done. Verify with 'uv --version' and 'python --version'."
