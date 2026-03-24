#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd -P)"
BIN_DIR="$REPO_ROOT/.bin"
DUNE_BIN_PATH="$BIN_DIR/dune"
LEGACY_SAND_BIN_PATH="$BIN_DIR/sand"
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
  if [ "$FORCE" -eq 1 ] || [ ! -x "$DUNE_BIN_PATH" ]; then
    return 0
  fi

  while IFS= read -r source_path; do
    if [ "$source_path" -nt "$DUNE_BIN_PATH" ]; then
      return 0
    fi
  done < <(
    find "$REPO_ROOT/cmd" "$REPO_ROOT/internal" -type f -name '*.go' | sort
    printf '%s\n' "$REPO_ROOT/go.mod" "$REPO_ROOT/go.sum"
  )

  return 1
}

if needs_rebuild; then
  echo "Building dune host binary..." >&2
  (
    cd "$REPO_ROOT"
    go build -o "$DUNE_BIN_PATH" ./cmd/dune
  )
fi

ln -sf "$DUNE_BIN_PATH" "$LEGACY_SAND_BIN_PATH"

if [ "$PRINT_PATH" -eq 1 ]; then
  printf '%s\n' "$DUNE_BIN_PATH"
else
  echo "dune binary ready at $DUNE_BIN_PATH" >&2
fi
