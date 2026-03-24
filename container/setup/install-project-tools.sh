#!/usr/bin/env bash
set -euo pipefail

# Optional extra tooling bootstrap. This script is safe to run repeatedly.
INSTALL_SYSTEM_TOOLS="${INSTALL_SYSTEM_TOOLS:-1}"
INSTALL_USER_TOOLS="${INSTALL_USER_TOOLS:-1}"

log() {
  echo "[install-project-tools] $*"
}

# Install gitui from GitHub releases (not available in Debian apt).
install_gitui() {
  if command -v gitui >/dev/null 2>&1; then
    log "gitui already installed"
    return 0
  fi

  # Resolve architecture
  ARCH=$(dpkg --print-architecture 2>/dev/null || uname -m)
  case "$ARCH" in
    amd64|x86_64)  GITUI_ARCH="x86_64" ;;
    arm64|aarch64) GITUI_ARCH="aarch64" ;;
    armhf|arm*)    GITUI_ARCH="arm" ;;
    *)
      log "Unsupported architecture '$ARCH', skipping gitui"
      return 0
      ;;
  esac

  log "Fetching latest gitui release for $GITUI_ARCH"
  GITUI_VERSION=$(curl -fsSL "https://api.github.com/repos/gitui-org/gitui/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"v\([^"]*\)".*/\1/')
  if [ -z "$GITUI_VERSION" ]; then
    log "Could not determine latest gitui version, skipping"
    return 0
  fi

  TMPDIR=$(mktemp -d)
  curl -fsSL "https://github.com/gitui-org/gitui/releases/download/v${GITUI_VERSION}/gitui-linux-${GITUI_ARCH}.tar.gz" \
    -o "$TMPDIR/gitui.tar.gz"
  tar -xzf "$TMPDIR/gitui.tar.gz" -C "$TMPDIR"

  INSTALL_CMD="install -m 755 $TMPDIR/gitui /usr/local/bin/gitui"
  if [ "$(id -u)" -eq 0 ]; then
    $INSTALL_CMD
  elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    sudo $INSTALL_CMD
  else
    log "No root/sudo access, skipping gitui install"
    rm -rf "$TMPDIR"
    return 0
  fi

  rm -rf "$TMPDIR"
  log "gitui ${GITUI_VERSION} installed"
}

# dolt - version-controlled database, required by beads
install_dolt() {
  if command -v dolt >/dev/null 2>&1; then
    log "dolt already installed"
    return 0
  fi
  log "Installing dolt"
  curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | sudo bash
}

# beads - memory system for agents
install_beads() {
  log "Installing beads"
  curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash
}

# beads viewer - TUI for beads
install_beads_viewer() {
  log "Installing beads_viewer"
  curl -fsSL "https://raw.githubusercontent.com/Dicklesworthstone/beads_viewer/main/install.sh?$(date +%s)" | bash
}

# mise - manage versions of node/python/go/rust/etc.
install_mise() {
  log "Installing mise"
  curl -fsSL https://mise.run | sh
}

if [ "$INSTALL_SYSTEM_TOOLS" = "1" ]; then
  install_gitui
fi

if [ "$INSTALL_USER_TOOLS" = "1" ]; then
  install_dolt
  install_beads
  install_beads_viewer
  install_mise
fi
