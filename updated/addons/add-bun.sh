#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-bun] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
BUN_VERSION="${SAND_BUN_VERSION:-latest}"

log() {
  echo "[add-bun] $*"
}

run_as_target_user() {
  runuser -u "$TARGET_USER" -- env \
    HOME="$TARGET_HOME" \
    USER="$TARGET_USER" \
    LOGNAME="$TARGET_USER" \
    PATH="${TARGET_HOME}/.local/bin:${TARGET_HOME}/.local/share/mise/shims:${PATH}" \
    "$@"
}

if ! run_as_target_user sh -lc 'command -v mise >/dev/null 2>&1'; then
  echo "[add-bun] mise is required but not found for ${TARGET_USER}" >&2
  exit 1
fi

log "Installing bun via mise (${BUN_VERSION})"
if [ "$BUN_VERSION" = "latest" ]; then
  run_as_target_user mise use -g bun@latest
else
  run_as_target_user mise use -g "bun@${BUN_VERSION}"
fi

run_as_target_user mise reshim

log "Done. Verify with 'bun --version' and 'bunx --version'."
