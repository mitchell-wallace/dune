#!/usr/bin/env bash
set -euo pipefail

HOME_DIR="${HOME_DIR:-/home/node}"
PERSIST_BASE="${PERSIST_BASE:-/persist/agent}"
DEFAULT_BASE="${DEFAULT_BASE:-/opt/agent-defaults}"

dir_has_entries() {
  local dir="$1"
  [ -d "$dir" ] && [ -n "$(find "$dir" -mindepth 1 -print -quit 2>/dev/null)" ]
}

seed_if_empty() {
  local agent="$1"
  local src="$2"
  local dst="$3"
  local default_dir="${DEFAULT_BASE}/${agent}"
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

link_home_dir() {
  local home_dir="$1"
  local persist_dir="$2"

  if [ -L "$home_dir" ]; then
    local target
    target="$(readlink "$home_dir")"
    if [ "$target" = "$persist_dir" ]; then
      return 0
    fi
  fi

  rm -rf "$home_dir"
  ln -s "$persist_dir" "$home_dir"
}

for agent in claude codex gemini; do
  home_dir="${HOME_DIR}/.${agent}"
  persist_dir="${PERSIST_BASE}/${agent}"
  seed_if_empty "$agent" "$home_dir" "$persist_dir"
  link_home_dir "$home_dir" "$persist_dir"
done
