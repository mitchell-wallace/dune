#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd -P)"
SAND_SCRIPT="$REPO_ROOT/sand.sh"
DUNE_SCRIPT="$REPO_ROOT/dune.sh"
BUILD_SCRIPT="$SCRIPT_DIR/build-sand.sh"

if [ ! -f "$SAND_SCRIPT" ]; then
  echo "Missing script: $SAND_SCRIPT" >&2
  exit 1
fi
if [ ! -f "$DUNE_SCRIPT" ]; then
  echo "Missing script: $DUNE_SCRIPT" >&2
  exit 1
fi

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "Missing build helper: $BUILD_SCRIPT" >&2
  exit 1
fi

"$BUILD_SCRIPT" >/dev/null

append_alias_if_missing() {
  local rc_file="$1"
  local alias_line="$2"

  touch "$rc_file"
  if grep -Fqx "$alias_line" "$rc_file"; then
    echo "Alias already present in $rc_file"
    return
  fi

  printf '\n%s\n' "$alias_line" >> "$rc_file"
  echo "Added alias to $rc_file"
}

append_alias_if_missing "$HOME/.bashrc" "alias dune='$DUNE_SCRIPT'"
append_alias_if_missing "$HOME/.bashrc" "alias sand='$SAND_SCRIPT'"
append_alias_if_missing "$HOME/.zshrc" "alias dune='$DUNE_SCRIPT'"
append_alias_if_missing "$HOME/.zshrc" "alias sand='$SAND_SCRIPT'"

echo "Done. Restart your shell or run: source ~/.bashrc (or source ~/.zshrc)"
