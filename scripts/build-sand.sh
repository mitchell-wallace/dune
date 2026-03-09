#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd -P)"
BIN_DIR="$REPO_ROOT/.bin"
BIN_PATH="$BIN_DIR/sand"
FORCE=0
PRINT_PATH=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --force)
      FORCE=1
      shift
      ;;
    --print-path)
      PRINT_PATH=1
      shift
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

mkdir -p "$BIN_DIR"

needs_rebuild() {
  if [ "$FORCE" -eq 1 ] || [ ! -x "$BIN_PATH" ]; then
    return 0
  fi

  while IFS= read -r source_path; do
    if [ "$source_path" -nt "$BIN_PATH" ]; then
      return 0
    fi
  done < <(
    find "$REPO_ROOT/cmd" "$REPO_ROOT/internal" -type f -name '*.go' | sort
    printf '%s\n' "$REPO_ROOT/go.mod" "$REPO_ROOT/go.sum"
  )

  return 1
}

if needs_rebuild; then
  echo "Building sand host binary..." >&2
  (
    cd "$REPO_ROOT"
    go build -o "$BIN_PATH" ./cmd/sand
  )
fi

if [ "$PRINT_PATH" -eq 1 ]; then
  printf '%s\n' "$BIN_PATH"
else
  echo "sand binary ready at $BIN_PATH" >&2
fi
