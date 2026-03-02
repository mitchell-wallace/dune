#!/usr/bin/env bash
set -euo pipefail

# Resolve this script's directory so it works no matter where it's launched from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CONFIG_FILE="$SCRIPT_DIR/updated/devcontainer.json"

usage() {
  cat >&2 <<'EOF'
Usage: sand [workspace_dir]

workspace_dir defaults to the current directory.
EOF
  exit 1
}

if [ "$#" -gt 1 ]; then
  usage
fi

WORKSPACE_INPUT="${1:-$PWD}"
if [ ! -d "$WORKSPACE_INPUT" ]; then
  echo "Workspace directory does not exist: $WORKSPACE_INPUT" >&2
  exit 1
fi

WORKSPACE_DIR="$(cd "$WORKSPACE_INPUT" && pwd -P)"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required but was not found in PATH." >&2
  exit 1
fi

if ! command -v npx >/dev/null 2>&1; then
  echo "npx is required but was not found in PATH." >&2
  exit 1
fi

if [ ! -f "$SCRIPT_DIR/updated/Dockerfile" ]; then
  echo "Expected Dockerfile at: $SCRIPT_DIR/updated/Dockerfile" >&2
  exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
  echo "Expected devcontainer.json at: $CONFIG_FILE" >&2
  exit 1
fi

PROJECT_BASENAME="$(basename "$WORKSPACE_DIR")"
PROJECT_SLUG="$(printf '%s' "$PROJECT_BASENAME" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-')"
PROJECT_HASH="$(printf '%s' "$WORKSPACE_DIR" | sha1sum | awk '{print substr($1,1,8)}')"

CONTAINER_NAME="sand-${PROJECT_SLUG}-${PROJECT_HASH}"

container_exists() {
  docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1
}

container_running() {
  [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null)" = "true" ]
}

if ! container_exists; then
  echo "Provisioning dev container via devcontainers CLI"
  (
    cd "$SCRIPT_DIR"
    npx @devcontainers/cli up --workspace-folder "$WORKSPACE_DIR" --config "$CONFIG_FILE"
  )

  # Find the container created by devcontainers for this workspace/config and
  # assign a stable, directory-derived name for future reuse.
  CREATED_ID="$(docker ps -aq \
    --filter "label=devcontainer.local_folder=$WORKSPACE_DIR" \
    --filter "label=devcontainer.config_file=$CONFIG_FILE" \
    | head -n1)"

  if [ -z "$CREATED_ID" ]; then
    echo "Could not find container created by devcontainers CLI." >&2
    exit 1
  fi

  docker rename "$CREATED_ID" "$CONTAINER_NAME" >/dev/null
fi

if ! container_running; then
  docker start "$CONTAINER_NAME" >/dev/null
fi

exec docker exec -it "$CONTAINER_NAME" zsh
