#!/usr/bin/env bash
set -euo pipefail

# Resolve this script's directory so it works no matter where it's launched from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CONFIG_FILE="$SCRIPT_DIR/updated/devcontainer.json"

usage() {
  cat >&2 <<'USAGE'
Usage: sand [workspace_dir] [profile] [mode]
       sand [profile] [mode]
       sand -d <workspace_dir> -p <profile> -m <mode>

Modes:
  std | standard (default)
  lax
  yolo
  strict

Profile:
  one character: 0-9 or a-z (case-insensitive), default: 0

Examples:
  sand
  sand 1
  sand strict
  sand 1 std
  sand ./my-project a lax
  sand -d ./strict -p 0 -m std
USAGE
  exit 1
}

canonicalize_mode() {
  local raw="$1"
  local mode
  mode="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  case "$mode" in
    std|standard)
      printf 'std\n'
      ;;
    lax|yolo|strict)
      printf '%s\n' "$mode"
      ;;
    *)
      return 1
      ;;
  esac
}

normalize_profile() {
  local raw="$1"
  local profile
  profile="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  if [[ "$profile" =~ ^[0-9a-z]$ ]]; then
    printf '%s\n' "$profile"
    return 0
  fi

  return 1
}

is_profile_token() {
  [[ "$1" =~ ^[0-9a-zA-Z]$ ]]
}

container_exists() {
  local name="$1"
  docker container inspect "$name" >/dev/null 2>&1
}

container_running() {
  local name="$1"
  [ "$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null)" = "true" ]
}

container_env_value() {
  local name="$1"
  local key="$2"

  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$name" 2>/dev/null \
    | awk -F= -v key="$key" '$1 == key { print $2; exit }'
}

resolve_container_mode() {
  local name="$1"
  local file_mode env_mode

  if container_running "$name"; then
    file_mode="$(docker exec "$name" sh -lc 'cat /etc/sand/security-mode 2>/dev/null || true' | tr -d '\r' | tr -d '\n')"
    if [ -n "$file_mode" ] && canonicalize_mode "$file_mode" >/dev/null 2>&1; then
      canonicalize_mode "$file_mode"
      return 0
    fi
  fi

  env_mode="$(container_env_value "$name" "SAND_SECURITY_MODE" || true)"
  if [ -n "$env_mode" ] && canonicalize_mode "$env_mode" >/dev/null 2>&1; then
    canonicalize_mode "$env_mode"
    return 0
  fi

  printf 'std\n'
}

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

workspace_input=""
profile=""
mode=""

positionals=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    -h|--help)
      usage
      ;;
    -d|--directory)
      if [ "$#" -lt 2 ]; then
        echo "Missing value for $1" >&2
        usage
      fi
      workspace_input="$2"
      shift 2
      ;;
    -p|--profile)
      if [ "$#" -lt 2 ]; then
        echo "Missing value for $1" >&2
        usage
      fi
      profile="$(normalize_profile "$2")" || {
        echo "Invalid profile '$2' (expected one char: 0-9 or a-z)" >&2
        exit 1
      }
      shift 2
      ;;
    -m|--mode)
      if [ "$#" -lt 2 ]; then
        echo "Missing value for $1" >&2
        usage
      fi
      mode="$(canonicalize_mode "$2")" || {
        echo "Invalid mode '$2' (expected: std|standard|lax|yolo|strict)" >&2
        exit 1
      }
      shift 2
      ;;
    --)
      shift
      while [ "$#" -gt 0 ]; do
        positionals+=("$1")
        shift
      done
      ;;
    -*)
      echo "Unknown option: $1" >&2
      usage
      ;;
    *)
      positionals+=("$1")
      shift
      ;;
  esac
done

for token in "${positionals[@]}"; do
  if [ -z "$profile" ] && is_profile_token "$token"; then
    profile="$(normalize_profile "$token")"
    continue
  fi

  if parsed_mode="$(canonicalize_mode "$token" 2>/dev/null)"; then
    mode="$parsed_mode"
    continue
  fi

  if [ -z "$workspace_input" ]; then
    workspace_input="$token"
    continue
  fi

  echo "Unexpected argument: $token" >&2
  usage
done

workspace_input="${workspace_input:-$PWD}"
profile="${profile:-0}"
mode="${mode:-std}"

if [ ! -d "$workspace_input" ]; then
  echo "Workspace directory does not exist: $workspace_input" >&2
  exit 1
fi

WORKSPACE_DIR="$(cd "$workspace_input" && pwd -P)"

PROJECT_BASENAME="$(basename "$WORKSPACE_DIR")"
PROJECT_SLUG="$(printf '%s' "$PROJECT_BASENAME" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-')"
PROJECT_HASH="$(printf '%s' "$WORKSPACE_DIR" | sha1sum | awk '{print substr($1,1,8)}')"

LEGACY_CONTAINER_NAME="sand-${PROJECT_SLUG}-${PROJECT_HASH}"
CONTAINER_NAME="sand-${PROJECT_SLUG}-${PROJECT_HASH}-${profile}"

if [ "$profile" = "0" ] && ! container_exists "$CONTAINER_NAME" && container_exists "$LEGACY_CONTAINER_NAME"; then
  echo "Migrating legacy container name: ${LEGACY_CONTAINER_NAME} -> ${CONTAINER_NAME}"
  docker rename "$LEGACY_CONTAINER_NAME" "$CONTAINER_NAME" >/dev/null
fi

if container_exists "$CONTAINER_NAME"; then
  existing_mode="$(resolve_container_mode "$CONTAINER_NAME")"
  if [ "$mode" != "$existing_mode" ]; then
    cat >&2 <<WARN_MSG
WARNING: Container '$CONTAINER_NAME' already exists with security mode '$existing_mode',
but you requested '$mode'. The existing container mode is immutable and will be used.
To change mode, remove/recreate this workspace+profile container.
WARN_MSG
  fi
else
  echo "Provisioning dev container via devcontainers CLI (profile=$profile mode=$mode)"
  (
    cd "$SCRIPT_DIR"
    SAND_PROFILE="$profile" \
      SAND_SECURITY_MODE="$mode" \
      npx @devcontainers/cli up \
        --workspace-folder "$WORKSPACE_DIR" \
        --config "$CONFIG_FILE" \
        --id-label "devcontainer.local_folder=$WORKSPACE_DIR" \
        --id-label "devcontainer.config_file=$CONFIG_FILE" \
        --id-label "sand.profile=$profile"
  )

  CREATED_ID="$(docker ps -aq \
    --filter "label=devcontainer.local_folder=$WORKSPACE_DIR" \
    --filter "label=devcontainer.config_file=$CONFIG_FILE" \
    --filter "label=sand.profile=$profile" \
    | head -n1)"

  if [ -z "$CREATED_ID" ]; then
    echo "Could not find container created by devcontainers CLI." >&2
    exit 1
  fi

  CREATED_NAME="$(docker inspect -f '{{.Name}}' "$CREATED_ID" | sed 's#^/##')"
  if [ "$CREATED_NAME" != "$CONTAINER_NAME" ]; then
    docker rename "$CREATED_ID" "$CONTAINER_NAME" >/dev/null
  fi
fi

if ! container_running "$CONTAINER_NAME"; then
  docker start "$CONTAINER_NAME" >/dev/null
fi

exec docker exec -it "$CONTAINER_NAME" zsh
