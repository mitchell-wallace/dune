#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-minio] must run as root" >&2
  exit 1
fi

log() {
  echo "[add-minio] $*"
}

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) release_arch="linux-amd64" ;;
  aarch64|arm64) release_arch="linux-arm64" ;;
  *)
    echo "[add-minio] unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

minio_url="https://dl.min.io/server/minio/release/${release_arch}/minio"
mc_url="https://dl.min.io/client/mc/release/${release_arch}/mc"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

log "Downloading MinIO server binary"
curl -fsSL "$minio_url" -o "$tmp_dir/minio"
install -m 0755 "$tmp_dir/minio" /usr/local/bin/minio

log "Downloading MinIO client binary (mc)"
curl -fsSL "$mc_url" -o "$tmp_dir/mc"
install -m 0755 "$tmp_dir/mc" /usr/local/bin/mc

chown root:root /usr/local/bin/minio /usr/local/bin/mc

log "Done. Example run: minio server --address 127.0.0.1:9000 --console-address 127.0.0.1:9001 /workspace/.minio-data"
