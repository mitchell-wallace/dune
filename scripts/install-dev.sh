#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
BUILD_SCRIPT="$SCRIPT_DIR/build-dune.sh"
DEST_DIR="${HOME}/.local/bin"
DEST_PATH="${DEST_DIR}/dunex"
ZSHRC="${HOME}/.zshrc"

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "Missing build helper: $BUILD_SCRIPT" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
DUNEX_BIN_PATH="$("$BUILD_SCRIPT" --force --print-path)"
install -m 0755 "$DUNEX_BIN_PATH" "$DEST_PATH"

ensure_path_in_zshrc() {
  local line="export PATH=\"\$HOME/.local/bin:\$PATH\""

  touch "$ZSHRC"
  if ! grep -Fqx "$line" "$ZSHRC"; then
    printf '\n%s\n' "$line" >> "$ZSHRC"
    echo "Added ~/.local/bin to PATH in $ZSHRC"
  fi
}

ensure_dx_alias() {
  local alias_line="alias dx='dunex'"

  touch "$ZSHRC"
  if grep -Eq "^[[:space:]]*alias[[:space:]]+dx=" "$ZSHRC"; then
    echo "dx alias already present in $ZSHRC"
    return
  fi

  printf '\n%s\n' "$alias_line" >> "$ZSHRC"
  echo "Added dx alias to $ZSHRC"
}

ensure_path_in_zshrc
ensure_dx_alias

"$DEST_PATH" version
echo "Installed dev dunex to $DEST_PATH"
echo "System dune was not modified."
