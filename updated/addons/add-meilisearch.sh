#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-meilisearch] must run as root" >&2
  exit 1
fi

log() {
  echo "[add-meilisearch] $*"
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

log "Installing Meilisearch via official installer"
(
  cd "$tmp_dir"
  curl -fsSL https://install.meilisearch.com | sh
)

if [ ! -f "$tmp_dir/meilisearch" ]; then
  echo "[add-meilisearch] meilisearch binary not produced by installer" >&2
  exit 1
fi

install -m 0755 "$tmp_dir/meilisearch" /usr/local/bin/meilisearch
chown root:root /usr/local/bin/meilisearch

log "Done. Example run: meilisearch --http-addr 127.0.0.1:7700"
