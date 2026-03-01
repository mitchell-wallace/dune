#!/usr/bin/env bash
set -euo pipefail

# Resolve this script's directory so it works no matter where it's launched from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required but was not found in PATH." >&2
  exit 1
fi

if [ ! -f "$PROJECT_DIR/updated/Dockerfile" ]; then
  echo "Expected Dockerfile at: $PROJECT_DIR/updated/Dockerfile" >&2
  exit 1
fi

if [ ! -f "$PROJECT_DIR/updated/init-firewall.sh" ]; then
  echo "Expected init-firewall.sh at: $PROJECT_DIR/updated/init-firewall.sh" >&2
  exit 1
fi

PROJECT_BASENAME="$(basename "$PROJECT_DIR")"
PROJECT_SLUG="$(printf '%s' "$PROJECT_BASENAME" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-')"
PROJECT_HASH="$(printf '%s' "$PROJECT_DIR" | sha1sum | awk '{print substr($1,1,8)}')"

IMAGE_NAME="sand-${PROJECT_SLUG}:${PROJECT_HASH}"
CONTAINER_NAME="sand-${PROJECT_SLUG}-${PROJECT_HASH}"
HISTORY_VOL="sand-history-${PROJECT_SLUG}-${PROJECT_HASH}"
CLAUDE_VOL="sand-claude-${PROJECT_SLUG}-${PROJECT_HASH}"

container_exists() {
  docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1
}

container_running() {
  [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null)" = "true" ]
}

if ! container_exists; then
  echo "Building image: $IMAGE_NAME"
  docker build -t "$IMAGE_NAME" -f "$PROJECT_DIR/updated/Dockerfile" "$PROJECT_DIR/updated"

  echo "Creating container: $CONTAINER_NAME"
  docker volume create "$HISTORY_VOL" >/dev/null
  docker volume create "$CLAUDE_VOL" >/dev/null

  docker run -d \
    --name "$CONTAINER_NAME" \
    --cap-add=NET_ADMIN \
    --cap-add=NET_RAW \
    -e NODE_OPTIONS="--max-old-space-size=4096" \
    -e CLAUDE_CONFIG_DIR="/home/node/.claude" \
    -e POWERLEVEL9K_DISABLE_GITSTATUS="true" \
    -v "$PROJECT_DIR:/workspace" \
    -v "$HISTORY_VOL:/commandhistory" \
    -v "$CLAUDE_VOL:/home/node/.claude" \
    "$IMAGE_NAME" \
    sleep infinity >/dev/null

  docker exec -u root "$CONTAINER_NAME" /usr/local/bin/init-firewall.sh
fi

if ! container_running; then
  docker start "$CONTAINER_NAME" >/dev/null
fi

exec docker exec -it "$CONTAINER_NAME" zsh
