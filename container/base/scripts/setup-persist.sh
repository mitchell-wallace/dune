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
  sudo chown -hR "${AGENT_USER}:${AGENT_GROUP}" "${dst}"
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

# Image-owned shell config (theme + rc). Symlink straight to the image defaults
# rather than seeding into the persist volume: seeding only copies when the
# destination is absent, so a profile created on an older image would shadow
# every future update with its stale copy. Linking to the defaults means version
# bumps reach existing profiles on the next start.
link_default() {
  local rel="$1"
  local home_path="${HOME_DIR}/${rel}"
  local default_path="${DEFAULTS_DIR}/${rel}"

  rm -rf "${home_path}"
  ensure_dir "$(dirname "${home_path}")"
  ln -s "${default_path}" "${home_path}"
}

remove_unwanted_skill() {
  local skill_name="$1"

  sudo rm -rf \
    "${PERSIST_DIR}/.claude/skills/${skill_name}" \
    "${PERSIST_DIR}/.codex/skills/${skill_name}"
}

link_persist_root() {
  if [ "${PERSIST_DIR}" = "/persist/agent" ]; then
    return 0
  fi

  case "${PERSIST_DIR}" in
    /*) ;;
    *)
      echo "PERSIST_DIR must be absolute: ${PERSIST_DIR}" >&2
      return 1
      ;;
  esac

  if [ ! -d "${PERSIST_DIR}" ]; then
    echo "PERSIST_DIR does not exist or is not a directory: ${PERSIST_DIR}" >&2
    return 1
  fi

  if [ -L /persist/agent ]; then
    sudo ln -sfnT "${PERSIST_DIR}" /persist/agent
    return 0
  fi

  if [ -e /persist/agent ]; then
    if [ -d /persist/agent ] && [ -z "$(find /persist/agent -mindepth 1 -print -quit 2>/dev/null)" ]; then
      sudo rmdir /persist/agent
    else
      echo "/persist/agent exists and is not an empty directory or symlink" >&2
      return 1
    fi
  fi

  sudo ln -sT "${PERSIST_DIR}" /persist/agent
}

ensure_persist_dir "${PERSIST_DIR}"
link_persist_root
sudo chown -hR "${AGENT_USER}:${AGENT_GROUP}" "${PERSIST_DIR}"
seed_dir ".claude"
seed_dir ".codex"
seed_dir ".config/opencode"
seed_dir ".local/share/opencode"
seed_dir ".config/gh"
seed_dir ".config/rally"
seed_file ".gitconfig"
seed_file ".git-credentials"
seed_file ".claude.json"

remove_unwanted_skill "bd-to-br-migration"

link_path ".claude" ".claude"
link_path ".codex" ".codex"
link_path ".config/opencode" ".config/opencode"
link_path ".local/share/opencode" ".local/share/opencode"
link_path ".config/gh" ".config/gh"
link_path ".config/rally" ".config/rally"
link_path ".gitconfig" ".gitconfig"
link_path ".git-credentials" ".git-credentials"
link_path ".claude.json" ".claude.json"

# Theme + rc are image-owned; link to the defaults so updates always apply.
link_default ".zshrc"
link_default ".p10k.zsh"

if [ ! -e "${HOME_DIR}/.agent-shell-setup.sh" ]; then
  cp -a "${DEFAULTS_DIR}/.agent-shell-setup.sh" "${HOME_DIR}/.agent-shell-setup.sh"
fi
