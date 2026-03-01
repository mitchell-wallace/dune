#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAND_SCRIPT="$SCRIPT_DIR/sand.sh"

if [ ! -f "$SAND_SCRIPT" ]; then
  echo "Missing script: $SAND_SCRIPT" >&2
  exit 1
fi

append_alias_if_missing() {
  local rc_file="$1"
  local alias_line
  alias_line="alias sand='$SAND_SCRIPT'"

  touch "$rc_file"
  if grep -Fqx "$alias_line" "$rc_file"; then
    echo "Alias already present in $rc_file"
    return
  fi

  printf '\n%s\n' "$alias_line" >> "$rc_file"
  echo "Added alias to $rc_file"
}

append_alias_if_missing "$HOME/.bashrc"
append_alias_if_missing "$HOME/.zshrc"

echo "Done. Restart your shell or run: source ~/.bashrc (or source ~/.zshrc)"
