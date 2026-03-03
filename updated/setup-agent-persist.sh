#!/usr/bin/env bash
set -euo pipefail

HOME_DIR="${HOME_DIR:-/home/node}"
PERSIST_BASE="${PERSIST_BASE:-/persist/agent}"
DEFAULT_BASE="${DEFAULT_BASE:-/opt/agent-defaults}"

dir_has_entries() {
  local dir="$1"
  [ -d "$dir" ] && [ -n "$(find "$dir" -mindepth 1 -print -quit 2>/dev/null)" ]
}

file_has_content() {
  local file="$1"
  [ -f "$file" ] && [ -s "$file" ]
}

link_home_path() {
  local home_path="$1"
  local persist_path="$2"

  if [ -L "$home_path" ]; then
    local target
    target="$(readlink "$home_path")"
    if [ "$target" = "$persist_path" ]; then
      return 0
    fi
  fi

  mkdir -p "$(dirname "$home_path")"
  rm -rf "$home_path"
  ln -s "$persist_path" "$home_path"
}

seed_dir_if_empty() {
  local key="$1"
  local src="$2"
  local dst="$3"
  local default_dir="${DEFAULT_BASE}/${key}"
  local source_is_link_to_dst=0

  mkdir -p "$dst"

  if ! dir_has_entries "$dst"; then
    if [ -L "$src" ] && [ "$(readlink "$src")" = "$dst" ]; then
      source_is_link_to_dst=1
    fi

    if [ "$source_is_link_to_dst" -eq 0 ] && dir_has_entries "$src"; then
      cp -a "$src/." "$dst/"
      return 0
    fi

    if dir_has_entries "$default_dir"; then
      cp -a "$default_dir/." "$dst/"
    fi
  fi
}

seed_file_if_empty() {
  local key="$1"
  local src="$2"
  local dst="$3"
  local default_file="${DEFAULT_BASE}/${key}"
  local source_is_link_to_dst=0

  mkdir -p "$(dirname "$dst")"

  if ! file_has_content "$dst"; then
    if [ -L "$src" ] && [ "$(readlink "$src")" = "$dst" ]; then
      source_is_link_to_dst=1
    fi

    if [ "$source_is_link_to_dst" -eq 0 ] && file_has_content "$src"; then
      cp -a "$src" "$dst"
      return 0
    fi

    if file_has_content "$default_file"; then
      cp -a "$default_file" "$dst"
    fi
  fi
}

persist_dir_mapping() {
  local key="$1"
  local home_dir="$2"
  local persist_dir="$3"

  seed_dir_if_empty "$key" "$home_dir" "$persist_dir"
  link_home_path "$home_dir" "$persist_dir"
}

persist_file_mapping() {
  local key="$1"
  local home_file="$2"
  local persist_file="$3"

  seed_file_if_empty "$key" "$home_file" "$persist_file"
  link_home_path "$home_file" "$persist_file"
}

persist_dir_mapping "claude" "${HOME_DIR}/.claude" "${PERSIST_BASE}/claude"
persist_dir_mapping "codex" "${HOME_DIR}/.codex" "${PERSIST_BASE}/codex"
persist_dir_mapping "gemini" "${HOME_DIR}/.gemini" "${PERSIST_BASE}/gemini"
persist_dir_mapping "opencode/config" "${HOME_DIR}/.config/opencode" "${PERSIST_BASE}/opencode/config"
persist_dir_mapping "opencode/data" "${HOME_DIR}/.local/share/opencode" "${PERSIST_BASE}/opencode/data"
persist_dir_mapping "gh/config" "${HOME_DIR}/.config/gh" "${PERSIST_BASE}/gh/config"

persist_file_mapping "git/.gitconfig" "${HOME_DIR}/.gitconfig" "${PERSIST_BASE}/git/.gitconfig"
persist_file_mapping "git/.git-credentials" "${HOME_DIR}/.git-credentials" "${PERSIST_BASE}/git/.git-credentials"
