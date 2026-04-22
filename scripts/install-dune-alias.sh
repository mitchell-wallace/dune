#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
BUILD_SCRIPT="$SCRIPT_DIR/build-dune.sh"

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "Missing build helper: $BUILD_SCRIPT" >&2
  exit 1
fi

DUNE_BIN_PATH="$("$BUILD_SCRIPT" --force --print-path)"

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

append_alias_if_missing "$HOME/.bashrc" "alias dune='$DUNE_BIN_PATH'"
append_alias_if_missing "$HOME/.zshrc" "alias dune='$DUNE_BIN_PATH'"

echo "Done. Restart your shell or run: source ~/.bashrc (or source ~/.zshrc)"
echo "Note: this alias points dune to the repo-local compiled Go binary and will override any standalone dune binary on your PATH."
