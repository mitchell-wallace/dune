#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-tmux] must run as root" >&2
  exit 1
fi

log() {
  echo "[add-tmux] $*"
}

log "Installing tmux package..."
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  tmux

log "Done."
