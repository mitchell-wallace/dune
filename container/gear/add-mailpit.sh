#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-mailpit] must run as root" >&2
  exit 1
fi

log() {
  echo "[add-mailpit] $*"
}

install_helper() {
  cat > /usr/local/bin/mp-local <<'EOF_HELPER'
#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -eq 0 ]; then
  exec /usr/local/bin/dune-privileged mp-local "$@"
fi

exec sudo /usr/local/bin/dune-privileged mp-local "$@"
EOF_HELPER
  chmod 0755 /usr/local/bin/mp-local
  chown root:root /usr/local/bin/mp-local
}

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) asset_name="mailpit-linux-amd64.tar.gz" ;;
  aarch64|arm64) asset_name="mailpit-linux-arm64.tar.gz" ;;
  *)
    echo "[add-mailpit] unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

log "Resolving latest Mailpit release metadata"
release_json="$(curl -fsSL https://api.github.com/repos/axllent/mailpit/releases/latest)"
asset_url="$(printf '%s' "$release_json" | jq -r --arg name "$asset_name" '.assets[] | select(.name == $name) | .browser_download_url' | head -n1)"

if [ -z "$asset_url" ] || [ "$asset_url" = "null" ]; then
  echo "[add-mailpit] unable to find download URL for asset: $asset_name" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

log "Downloading $asset_name"
curl -fsSL "$asset_url" -o "$tmp_dir/mailpit.tar.gz"
tar -xzf "$tmp_dir/mailpit.tar.gz" -C "$tmp_dir"

if [ ! -f "$tmp_dir/mailpit" ]; then
  echo "[add-mailpit] mailpit binary not found in archive" >&2
  exit 1
fi

install -m 0755 "$tmp_dir/mailpit" /usr/local/bin/mailpit
chown root:root /usr/local/bin/mailpit

install_helper
log "Starting Mailpit"
/usr/local/bin/dune-privileged mp-local start >/dev/null
log "Done. Example run: mp-local start"
