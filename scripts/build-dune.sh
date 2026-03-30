#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd -P)"
BIN_DIR="$REPO_ROOT/.bin"
DUNE_BIN_PATH="$BIN_DIR/dune"
VERSION_FILE="$REPO_ROOT/VERSION"
IMAGE_VERSION_FILE="$REPO_ROOT/container/base/IMAGE_VERSION"
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

if [ ! -f "$VERSION_FILE" ]; then
  echo "Missing version file: $VERSION_FILE" >&2
  exit 1
fi

if [ ! -f "$IMAGE_VERSION_FILE" ]; then
  echo "Missing image version file: $IMAGE_VERSION_FILE" >&2
  exit 1
fi

needs_rebuild() {
  if [ "$FORCE" -eq 1 ] || [ ! -x "$DUNE_BIN_PATH" ]; then
    return 0
  fi

  while IFS= read -r source_path; do
    if [ "$source_path" -nt "$DUNE_BIN_PATH" ]; then
      return 0
    fi
  done < <(
    find \
      "$REPO_ROOT/cmd/dune" \
      "$REPO_ROOT/internal/dune" \
      "$REPO_ROOT/internal/version" \
      -type f \
      \( -name '*.go' -o -name '*.tmpl' \) | sort
    printf '%s\n' "$REPO_ROOT/go.mod" "$REPO_ROOT/go.sum" "$VERSION_FILE" "$IMAGE_VERSION_FILE"
  )

  return 1
}

if needs_rebuild; then
  echo "Building dune host binary..." >&2
  DUNE_VERSION="$(tr -d '\n' < "$VERSION_FILE")"
  BASE_IMAGE_VERSION="$(tr -d '\n' < "$IMAGE_VERSION_FILE")"
  DUNE_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
  (
    cd "$REPO_ROOT"
    go build \
      -ldflags "\
        -X claudebox/internal/version.Version=$DUNE_VERSION \
        -X claudebox/internal/version.Commit=$DUNE_COMMIT \
        -X claudebox/internal/version.BaseImageVersion=$BASE_IMAGE_VERSION" \
      -o "$DUNE_BIN_PATH" \
      ./cmd/dune
  )
fi

if [ "$PRINT_PATH" -eq 1 ]; then
  printf '%s\n' "$DUNE_BIN_PATH"
else
  echo "dune binary ready at $DUNE_BIN_PATH" >&2
fi
