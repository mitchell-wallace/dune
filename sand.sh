#!/usr/bin/env bash
set -euo pipefail

# Resolve this script's directory so it works no matter where it's launched from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CONFIG_FILE="$SCRIPT_DIR/updated/devcontainer.json"

SAND_CONFIG_PATH=""
CONFIG_PROFILE=""
CONFIG_MODE=""
CONFIG_ADDONS=()
CONFIG_PYTHON_VERSION=""
CONFIG_UV_VERSION=""
CONFIG_GO_VERSION=""
CONFIG_RUST_VERSION=""
CONFIG_DOTNET_VERSION=""
CONFIG_JAVA_VERSION=""
CONFIG_MAVEN_VERSION=""
CONFIG_GRADLE_VERSION=""
CONFIG_BUN_VERSION=""
CONFIG_DENO_VERSION=""

usage() {
  cat >&2 <<'USAGE'
Usage: sand [workspace_dir] [profile] [mode]
       sand [profile] [mode]
       sand -d <workspace_dir> -p <profile> -m <mode>
       sand config [-d <workspace_dir>]

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
  sand config
  sand config -d ./repo

Notes:
  - 'sand config' is an interactive sand.toml wizard
  - to target a folder literally named 'config', run: sand -d ./config
USAGE
  exit 1
}

config_usage() {
  cat <<'USAGE'
Usage: sand config [-d <workspace_dir>]

Interactive wizard that creates/updates sand.toml at the workspace git root.

Options:
  -d, --directory  workspace directory (default: current directory)
  -h, --help       show this help
USAGE
}

warn() {
  echo "WARNING: $*" >&2
}

run_config_wizard() {
  local workspace_input="${PWD}"
  local positional_dir=""

  while [ "$#" -gt 0 ]; do
    case "$1" in
      -h|--help)
        config_usage
        return 0
        ;;
      -d|--directory)
        if [ "$#" -lt 2 ]; then
          echo "Missing value for $1" >&2
          config_usage >&2
          return 1
        fi
        workspace_input="$2"
        shift 2
        ;;
      -*)
        echo "Unknown option for sand config: $1" >&2
        config_usage >&2
        return 1
        ;;
      *)
        if [ -z "$positional_dir" ]; then
          positional_dir="$1"
          shift
        else
          echo "Unexpected argument for sand config: $1" >&2
          config_usage >&2
          return 1
        fi
        ;;
    esac
  done

  if [ -n "$positional_dir" ]; then
    workspace_input="$positional_dir"
  fi

  if [ ! -t 0 ] || [ ! -t 1 ]; then
    echo "'sand config' requires an interactive terminal (TTY)." >&2
    return 1
  fi

  if ! command -v uv >/dev/null 2>&1; then
    echo "uv is required for 'sand config' but was not found in PATH." >&2
    echo "Install uv: https://docs.astral.sh/uv/getting-started/installation/" >&2
    return 1
  fi

  if [ ! -d "$workspace_input" ]; then
    echo "Workspace directory does not exist: $workspace_input" >&2
    return 1
  fi

  local workspace_dir wizard_project manifest_path
  workspace_dir="$(cd "$workspace_input" && pwd -P)"
  wizard_project="$SCRIPT_DIR/tools/sand-config"
  manifest_path="$SCRIPT_DIR/updated/addons/manifest.tsv"

  if [ ! -f "$wizard_project/pyproject.toml" ]; then
    echo "Missing sand-config project: $wizard_project/pyproject.toml" >&2
    return 1
  fi

  if [ ! -f "$manifest_path" ]; then
    echo "Missing addons manifest: $manifest_path" >&2
    return 1
  fi

  exec uv run --project "$wizard_project" sand-config \
    --directory "$workspace_dir" \
    --manifest "$manifest_path"
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

find_sand_toml() {
  local base_dir="$1"
  local git_root=""
  local search_dir="$base_dir"
  local depth=0

  if command -v git >/dev/null 2>&1; then
    git_root="$(git -C "$base_dir" rev-parse --show-toplevel 2>/dev/null || true)"
    if [ -n "$git_root" ] && [ -f "$git_root/sand.toml" ]; then
      printf '%s\n' "$git_root/sand.toml"
      return 0
    fi
  fi

  while [ "$depth" -le 5 ]; do
    if [ -f "$search_dir/sand.toml" ]; then
      printf '%s\n' "$search_dir/sand.toml"
      return 0
    fi

    if [ "$search_dir" = "/" ]; then
      break
    fi

    search_dir="$(dirname "$search_dir")"
    depth=$((depth + 1))
  done

  return 1
}

parse_sand_toml() {
  local path="$1"
  local parsed=""
  local line_type key value

  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to parse sand.toml: $path" >&2
    exit 1
  fi

  parsed="$(python3 - "$path" <<'PY'
import sys
import tomllib

path = sys.argv[1]
allowed = {
    "profile",
    "mode",
    "addons",
    "python_version",
    "uv_version",
    "go_version",
    "rust_version",
    "dotnet_version",
    "java_version",
    "maven_version",
    "gradle_version",
    "bun_version",
    "deno_version",
}

with open(path, "rb") as f:
    data = tomllib.load(f)

if not isinstance(data, dict):
    print("error\troot\tmust be a table")
    sys.exit(2)

for k in data.keys():
    if k not in allowed:
        print(f"unknown\t{k}\t")

for scalar_key in [
    "profile",
    "mode",
    "python_version",
    "uv_version",
    "go_version",
    "rust_version",
    "dotnet_version",
    "java_version",
    "maven_version",
    "gradle_version",
    "bun_version",
    "deno_version",
]:
    value = data.get(scalar_key)
    if value is None:
        continue
    if not isinstance(value, str):
        print(f"error\t{scalar_key}\texpected string")
        sys.exit(2)
    print(f"scalar\t{scalar_key}\t{value}")

addons = data.get("addons")
if addons is not None:
    if not isinstance(addons, list) or not all(isinstance(x, str) for x in addons):
        print("error\taddons\texpected array of strings")
        sys.exit(2)
    for addon in addons:
        print(f"addon\t{addon}\t")
PY
)" || {
    echo "Failed to parse sand.toml: $path" >&2
    exit 1
  }

  while IFS=$'\t' read -r line_type key value; do
    case "$line_type" in
      unknown)
        warn "Unknown key in sand.toml ignored: $key"
        ;;
      scalar)
        case "$key" in
          profile) CONFIG_PROFILE="$value" ;;
          mode) CONFIG_MODE="$value" ;;
          python_version) CONFIG_PYTHON_VERSION="$value" ;;
          uv_version) CONFIG_UV_VERSION="$value" ;;
          go_version) CONFIG_GO_VERSION="$value" ;;
          rust_version) CONFIG_RUST_VERSION="$value" ;;
          dotnet_version) CONFIG_DOTNET_VERSION="$value" ;;
          java_version) CONFIG_JAVA_VERSION="$value" ;;
          maven_version) CONFIG_MAVEN_VERSION="$value" ;;
          gradle_version) CONFIG_GRADLE_VERSION="$value" ;;
          bun_version) CONFIG_BUN_VERSION="$value" ;;
          deno_version) CONFIG_DENO_VERSION="$value" ;;
        esac
        ;;
      addon)
        CONFIG_ADDONS+=("$key")
        ;;
      error)
        echo "Invalid sand.toml ($key): $value" >&2
        exit 1
        ;;
      "")
        ;;
      *)
        warn "Unexpected parser output in sand.toml ignored: $line_type"
        ;;
    esac
  done <<<"$parsed"
}

build_addon_env_args() {
  local -n out_ref="$1"
  out_ref=()

  [ -n "$CONFIG_PYTHON_VERSION" ] && out_ref+=("-e" "SAND_PYTHON_VERSION=$CONFIG_PYTHON_VERSION")
  [ -n "$CONFIG_UV_VERSION" ] && out_ref+=("-e" "SAND_UV_VERSION=$CONFIG_UV_VERSION")
  [ -n "$CONFIG_GO_VERSION" ] && out_ref+=("-e" "SAND_GO_VERSION=$CONFIG_GO_VERSION")
  [ -n "$CONFIG_RUST_VERSION" ] && out_ref+=("-e" "SAND_RUST_VERSION=$CONFIG_RUST_VERSION")
  [ -n "$CONFIG_DOTNET_VERSION" ] && out_ref+=("-e" "SAND_DOTNET_VERSION=$CONFIG_DOTNET_VERSION")
  [ -n "$CONFIG_JAVA_VERSION" ] && out_ref+=("-e" "SAND_JAVA_VERSION=$CONFIG_JAVA_VERSION")
  [ -n "$CONFIG_MAVEN_VERSION" ] && out_ref+=("-e" "SAND_MAVEN_VERSION=$CONFIG_MAVEN_VERSION")
  [ -n "$CONFIG_GRADLE_VERSION" ] && out_ref+=("-e" "SAND_GRADLE_VERSION=$CONFIG_GRADLE_VERSION")
  [ -n "$CONFIG_BUN_VERSION" ] && out_ref+=("-e" "SAND_BUN_VERSION=$CONFIG_BUN_VERSION")
  [ -n "$CONFIG_DENO_VERSION" ] && out_ref+=("-e" "SAND_DENO_VERSION=$CONFIG_DENO_VERSION")
}

apply_configured_addons() {
  local container_name="$1"
  local effective_mode="$2"
  local known_addons=""
  local addon=""
  local installed_count=0
  local skipped_installed_count=0
  local skipped_unknown_count=0
  local skipped_invalid_count=0
  local -a addon_env_args

  if [ "${#CONFIG_ADDONS[@]}" -eq 0 ]; then
    return 0
  fi

  if [ "$effective_mode" = "strict" ]; then
    warn "sand.toml lists addons but mode is strict; ignoring configured addons."
    return 0
  fi

  known_addons="$(docker exec "$container_name" awk -F'\t' 'NR>1 && $1 != "" { print $1 }' /usr/local/lib/sand/addons/manifest.tsv 2>/dev/null || true)"
  if [ -z "$known_addons" ]; then
    echo "ERROR: Unable to load addon manifest from container '$container_name'." >&2
    return 1
  fi

  build_addon_env_args addon_env_args

  echo "Applying configured addons from sand.toml (${#CONFIG_ADDONS[@]} requested)..."
  for addon in "${CONFIG_ADDONS[@]}"; do
    if [[ ! "$addon" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
      warn "Invalid addon name in sand.toml skipped: $addon"
      skipped_invalid_count=$((skipped_invalid_count + 1))
      continue
    fi

    if ! printf '%s\n' "$known_addons" | grep -Fxq "$addon"; then
      warn "Unknown addon in sand.toml skipped: $addon"
      skipped_unknown_count=$((skipped_unknown_count + 1))
      continue
    fi

    if docker exec "$container_name" sh -lc "[ -f '/persist/agent/addons/${addon}.installed' ]" >/dev/null 2>&1; then
      skipped_installed_count=$((skipped_installed_count + 1))
      continue
    fi

    echo "Installing addon from sand.toml: $addon"
    if ! docker exec "${addon_env_args[@]}" "$container_name" addons "$addon"; then
      echo "ERROR: Failed to install configured addon '$addon'" >&2
      return 1
    fi

    installed_count=$((installed_count + 1))
  done

  echo "sand.toml addon summary: installed=$installed_count skipped_installed=$skipped_installed_count skipped_unknown=$skipped_unknown_count skipped_invalid=$skipped_invalid_count"
}

if [ "${1:-}" = "config" ]; then
  shift
  run_config_wizard "$@"
  exit $?
fi

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
profile_explicit=0
mode_explicit=0

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
      profile_explicit=1
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
      mode_explicit=1
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
  if [ "$profile_explicit" -eq 0 ] && [ -z "$profile" ] && is_profile_token "$token"; then
    profile="$(normalize_profile "$token")"
    profile_explicit=1
    continue
  fi

  if parsed_mode="$(canonicalize_mode "$token" 2>/dev/null)"; then
    mode="$parsed_mode"
    mode_explicit=1
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

if SAND_CONFIG_PATH="$(find_sand_toml "$WORKSPACE_DIR" 2>/dev/null)"; then
  parse_sand_toml "$SAND_CONFIG_PATH"
  echo "Using sand.toml config: $SAND_CONFIG_PATH"
fi

if [ "$profile_explicit" -eq 0 ] && [ -n "$CONFIG_PROFILE" ]; then
  profile="$(normalize_profile "$CONFIG_PROFILE")" || {
    echo "Invalid profile in sand.toml: '$CONFIG_PROFILE' (expected one char: 0-9 or a-z)" >&2
    exit 1
  }
fi

if [ "$mode_explicit" -eq 0 ] && [ -n "$CONFIG_MODE" ]; then
  mode="$(canonicalize_mode "$CONFIG_MODE")" || {
    echo "Invalid mode in sand.toml: '$CONFIG_MODE' (expected: std|standard|lax|yolo|strict)" >&2
    exit 1
  }
fi

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
  mode="$existing_mode"
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

apply_configured_addons "$CONTAINER_NAME" "$mode"

exec docker exec -it "$CONTAINER_NAME" zsh
