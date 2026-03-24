#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CALLER_PWD="${PWD}"
BUILD_SCRIPT="$SCRIPT_DIR/scripts/build-dune.sh"

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "Missing build helper: $BUILD_SCRIPT" >&2
  exit 1
fi

BINARY_PATH="$("$BUILD_SCRIPT" --print-path)"

exec env \
  DUNE_REPO_ROOT="$SCRIPT_DIR" \
  DUNE_CALLER_PWD="$CALLER_PWD" \
  "$BINARY_PATH" "$@"
