#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CALLER_PWD="${PWD}"

cd "$SCRIPT_DIR"
exec env \
  SAND_REPO_ROOT="$SCRIPT_DIR" \
  SAND_CALLER_PWD="$CALLER_PWD" \
  go run ./cmd/sand "$@"
