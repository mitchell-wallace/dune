#!/usr/bin/env bash
set -euo pipefail

TARGET_USER="${DUNE_TARGET_USER:-node}"
TARGET_HOME="${DUNE_TARGET_HOME:-/home/${TARGET_USER}}"

log() {
  echo "[boost-cli] $*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi

  if need_cmd sudo; then
    sudo "$@"
    return
  fi

  log "ERROR: sudo is required to install apt packages"
  exit 1
}

ensure_local_bin() {
  mkdir -p "${TARGET_HOME}/.local/bin"
  chown "${TARGET_USER}:${TARGET_USER}" "${TARGET_HOME}/.local" "${TARGET_HOME}/.local/bin"
}

ensure_symlink() {
  local target="$1"
  local link="$2"
  if [ ! -e "$link" ]; then
    ln -s "$target" "$link"
    chown -h "${TARGET_USER}:${TARGET_USER}" "$link"
  fi
}

install_apt_basics() {
  log "Installing modern CLI tools via apt (fd-find, ripgrep, bat, tree, fzf)"
  run_as_root apt-get update
  run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    fd-find \
    ripgrep \
    bat \
    tree \
    fzf

  ensure_local_bin
  if ! need_cmd fd && need_cmd fdfind; then
    ensure_symlink "$(command -v fdfind)" "${TARGET_HOME}/.local/bin/fd"
  fi
  if ! need_cmd bat && need_cmd batcat; then
    ensure_symlink "$(command -v batcat)" "${TARGET_HOME}/.local/bin/bat"
  fi
  if ! need_cmd tre && need_cmd tree; then
    ensure_symlink "$(command -v tree)" "${TARGET_HOME}/.local/bin/tre"
  fi
}

install_eza() {
  if need_cmd eza; then
    log "eza already installed"
    return
  fi

  log "Attempting eza install via apt"
  if run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends eza; then
    return
  fi

  log "eza not available via apt; falling back to official GitHub release binary"

  local arch
  case "$(uname -m)" in
    x86_64) arch="x86_64" ;;
    aarch64|arm64) arch="aarch64" ;;
    *)
      log "WARNING: unsupported architecture for eza fallback: $(uname -m)"
      return
      ;;
  esac

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  local release_json
  release_json="$tmp_dir/release.json"
  curl -fsSL https://api.github.com/repos/eza-community/eza/releases/latest > "$release_json"

  local asset_url
  asset_url="$(jq -r --arg arch "$arch" '.assets[] | select(.name | test($arch + "-unknown-linux-gnu.tar.gz$")) | .browser_download_url' "$release_json" | head -n1)"

  if [ -z "$asset_url" ]; then
    log "WARNING: unable to find eza release asset for $arch"
    rm -rf "$tmp_dir"
    return
  fi

  curl -fsSL "$asset_url" -o "$tmp_dir/eza.tar.gz"
  tar -xzf "$tmp_dir/eza.tar.gz" -C "$tmp_dir"

  local eza_bin
  eza_bin="$(find "$tmp_dir" -type f -name eza | head -n1)"
  if [ -z "$eza_bin" ]; then
    log "WARNING: eza binary not found after extraction"
    rm -rf "$tmp_dir"
    return
  fi

  ensure_local_bin
  install -m 0755 "$eza_bin" "${TARGET_HOME}/.local/bin/eza"
  chown "${TARGET_USER}:${TARGET_USER}" "${TARGET_HOME}/.local/bin/eza"
  rm -rf "$tmp_dir"
}

install_micro() {
  if need_cmd micro; then
    log "micro already installed"
    return
  fi

  log "Installing micro via official installer"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  (
    cd "$tmp_dir"
    curl -fsSL https://getmic.ro | bash
  )

  if [ ! -f "$tmp_dir/micro" ]; then
    log "WARNING: micro installer did not produce binary"
    rm -rf "$tmp_dir"
    return
  fi

  ensure_local_bin
  install -m 0755 "$tmp_dir/micro" "${TARGET_HOME}/.local/bin/micro"
  chown "${TARGET_USER}:${TARGET_USER}" "${TARGET_HOME}/.local/bin/micro"
  rm -rf "$tmp_dir"
}

log "Starting optional CLI boost install"
install_apt_basics
install_eza
install_micro
log "Done. Restart shell or run: hash -r"
