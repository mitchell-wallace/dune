#!/usr/bin/env bash
set -euo pipefail

HOME_DIR="${HOME_DIR:-/home/agent}"
PERSIST_DIR="${PERSIST_DIR:-/persist/agent}"
DEFAULTS_DIR="${DEFAULTS_DIR:-/opt/home-defaults}"
AGENT_USER="${AGENT_USER:-$(id -un)}"
AGENT_GROUP="${AGENT_GROUP:-$(id -gn)}"

ensure_dir() {
  install -d -m 0755 "${1}"
}

ensure_persist_dir() {
  local path="$1"
  sudo install -d -o "${AGENT_USER}" -g "${AGENT_GROUP}" -m 0755 "${path}"
}

copy_to_persist() {
  local src="$1"
  local dst="$2"

  sudo cp -a "${src}" "${dst}"
  sudo chown -R "${AGENT_USER}:${AGENT_GROUP}" "${dst}"
}

seed_dir() {
  local rel="$1"
  local dst="${PERSIST_DIR}/${rel}"
  local src="${DEFAULTS_DIR}/${rel}"

  ensure_persist_dir "${dst}"
  if [ -d "${src}" ] && [ -z "$(find "${dst}" -mindepth 1 -print -quit 2>/dev/null)" ]; then
    copy_to_persist "${src}/." "${dst}/"
  fi
}

seed_file() {
  local rel="$1"
  local dst="${PERSIST_DIR}/${rel}"
  local src="${DEFAULTS_DIR}/${rel}"

  ensure_persist_dir "$(dirname "${dst}")"
  if [ ! -e "${dst}" ] && [ -e "${src}" ]; then
    copy_to_persist "${src}" "${dst}"
  fi
}

link_path() {
  local home_rel="$1"
  local persist_rel="$2"
  local home_path="${HOME_DIR}/${home_rel}"
  local persist_path="${PERSIST_DIR}/${persist_rel}"

  rm -rf "${home_path}"
  ensure_dir "$(dirname "${home_path}")"
  ln -s "${persist_path}" "${home_path}"
}

ensure_persist_dir "${PERSIST_DIR}"
sudo chown -R "${AGENT_USER}:${AGENT_GROUP}" "${PERSIST_DIR}"
seed_dir ".claude"
seed_dir ".codex"
seed_dir ".config/opencode"
seed_dir ".local/share/opencode"
seed_dir ".config/gh"
seed_file ".gitconfig"
seed_file ".git-credentials"
seed_file ".zshrc"
seed_file ".p10k.zsh"

link_path ".claude" ".claude"
link_path ".codex" ".codex"
link_path ".config/opencode" ".config/opencode"
link_path ".local/share/opencode" ".local/share/opencode"
link_path ".config/gh" ".config/gh"
link_path ".gitconfig" ".gitconfig"
link_path ".git-credentials" ".git-credentials"
link_path ".zshrc" ".zshrc"
link_path ".p10k.zsh" ".p10k.zsh"

if [ ! -e "${HOME_DIR}/.agent-shell-setup.sh" ]; then
  cp -a "${DEFAULTS_DIR}/.agent-shell-setup.sh" "${HOME_DIR}/.agent-shell-setup.sh"
fi
