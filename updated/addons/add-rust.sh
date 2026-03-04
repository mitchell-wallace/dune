#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-rust] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
RUST_VERSION="${SAND_RUST_VERSION:-stable}"

log() {
  echo "[add-rust] $*"
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
  echo "[add-rust] mise is required but not found for ${TARGET_USER}" >&2
  exit 1
fi

log "Installing rust via mise (${RUST_VERSION})"
if [ "$RUST_VERSION" = "latest" ]; then
  run_as_target_user mise use -g rust@latest
else
  run_as_target_user mise use -g "rust@${RUST_VERSION}"
fi

run_as_target_user mise reshim

log "Done. Verify with 'rustc --version' and 'cargo --version'."
