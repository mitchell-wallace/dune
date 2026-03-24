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

seed_path_if_empty() {
  local mapping_type="$1"
  local key="$2"
  local src="$3"
  local dst="$4"
  local default_path="${DEFAULT_BASE}/${key}"
  local source_is_link_to_dst=0

  case "$mapping_type" in
    dir|seed-only-dir)
      mkdir -p "$dst"
      if dir_has_entries "$dst"; then
        return 0
      fi

      if [ "$mapping_type" = "dir" ] && [ -L "$src" ] && [ "$(readlink "$src")" = "$dst" ]; then
        source_is_link_to_dst=1
      fi

      if [ "$mapping_type" = "dir" ] && [ "$source_is_link_to_dst" -eq 0 ] && dir_has_entries "$src"; then
        cp -a "$src/." "$dst/"
        return 0
      fi

      if dir_has_entries "$default_path"; then
        cp -a "$default_path/." "$dst/"
      fi
      ;;
    file)
      mkdir -p "$(dirname "$dst")"
      if file_has_content "$dst"; then
        return 0
      fi

      if [ -L "$src" ] && [ "$(readlink "$src")" = "$dst" ]; then
        source_is_link_to_dst=1
      fi

      if [ "$source_is_link_to_dst" -eq 0 ] && file_has_content "$src"; then
        cp -a "$src" "$dst"
        return 0
      fi

      if file_has_content "$default_path"; then
        cp -a "$default_path" "$dst"
      fi
      ;;
    *)
      echo "Unsupported mapping type: ${mapping_type}" >&2
      exit 1
      ;;
  esac
}

apply_mapping() {
  local mapping_type="$1"
  local key="$2"
  local home_path="$3"
  local persist_path="$4"

  seed_path_if_empty "$mapping_type" "$key" "$home_path" "$persist_path"

  case "$mapping_type" in
    dir|file)
      link_home_path "$home_path" "$persist_path"
      ;;
    seed-only-dir)
      ;;
  esac
}

ensure_rally_binary() {
  local persist_bin="${PERSIST_BASE}/rally/bin/rally"
  local target_path="/usr/local/bin/rally"

  mkdir -p "$(dirname "$persist_bin")"

  if [ ! -x "$persist_bin" ]; then
    if ! chmod 0755 "$persist_bin" 2>/dev/null; then
      if [ ! -x "$persist_bin" ]; then
        echo "WARNING: ${persist_bin} exists but is not executable, and its mode could not be updated" >&2
        return 1
      fi
    fi
  fi

  if [ -L "$target_path" ] && [ "$(readlink "$target_path")" = "$persist_bin" ]; then
    return 0
  fi

  if grep -Fq " ${target_path} " /proc/self/mountinfo 2>/dev/null; then
    echo "WARNING: ${target_path} is a mounted path; rebuild this container once to switch rally to the persisted system binary" >&2
    return 0
  fi

  rm -f "$target_path"
  ln -s "$persist_bin" "$target_path"
}

PERSIST_MAPPINGS=(
  "dir|claude|${HOME_DIR}/.claude|${PERSIST_BASE}/claude"
  "dir|codex|${HOME_DIR}/.codex|${PERSIST_BASE}/codex"
  "dir|gemini|${HOME_DIR}/.gemini|${PERSIST_BASE}/gemini"
  "dir|opencode/config|${HOME_DIR}/.config/opencode|${PERSIST_BASE}/opencode/config"
  "dir|opencode/data|${HOME_DIR}/.local/share/opencode|${PERSIST_BASE}/opencode/data"
  "dir|gh/config|${HOME_DIR}/.config/gh|${PERSIST_BASE}/gh/config"
  "file|git/.gitconfig|${HOME_DIR}/.gitconfig|${PERSIST_BASE}/git/.gitconfig"
  "file|git/.git-credentials|${HOME_DIR}/.git-credentials|${PERSIST_BASE}/git/.git-credentials"
  "seed-only-dir|gear|-|${PERSIST_BASE}/gear"
)

for mapping in "${PERSIST_MAPPINGS[@]}"; do
  IFS='|' read -r mapping_type key home_path persist_path <<<"$mapping"
  apply_mapping "$mapping_type" "$key" "$home_path" "$persist_path"
done

ensure_rally_binary
