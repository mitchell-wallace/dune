#!/usr/bin/env bash
set -euo pipefail

# Resolve this script's directory so it works no matter where it's launched from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
CONFIG_FILE="$SCRIPT_DIR/updated/devcontainer.json"

SAND_CONFIG_PATH=""
CONFIG_PROFILE=""
CONFIG_MODE=""
CONFIG_WORKSPACE_MODE=""
CONFIG_ADDONS=()
CONFIG_PYTHON_VERSION=""
CONFIG_UV_VERSION=""
CONFIG_GO_VERSION=""
CONFIG_RUST_VERSION=""

usage() {
  cat >&2 <<'USAGE'
Usage: sand [workspace_dir] [profile] [mode]
       sand [profile] [mode]
       sand -d <workspace_dir> -p <profile> -m <mode>
       sand config [-d <workspace_dir>]
       sand rebuild [-d <workspace_dir>]

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
  sand rebuild
  sand rebuild -d ./repo

Notes:
  - 'sand config' is an interactive sand.toml wizard
  - 'sand rebuild' tears down and rebuilds the container for the workspace
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

ensure_sand_config_binary() {
  local wizard_project="$SCRIPT_DIR/tools/sand-config"
  local binary_dir="$wizard_project/.bin"
  local binary_path="$binary_dir/sand-config"
  local source_path=""

  if ! command -v mise >/dev/null 2>&1; then
    echo "mise is required for sand config but was not found in PATH." >&2
    echo "Install mise: https://mise.jdx.dev/getting-started.html" >&2
    return 1
  fi

  if [ ! -f "$wizard_project/go.mod" ]; then
    echo "Missing sand-config Go module: $wizard_project/go.mod" >&2
    return 1
  fi

  mkdir -p "$binary_dir"

  if [ ! -x "$binary_path" ]; then
    :
  elif [ -f "$SCRIPT_DIR/.mise.toml" ] && [ "$SCRIPT_DIR/.mise.toml" -nt "$binary_path" ]; then
    :
  else
    while IFS= read -r source_path; do
      if [ "$source_path" -nt "$binary_path" ]; then
        break
      fi
      source_path=""
    done < <(find "$wizard_project" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) | sort)
  fi

  if [ ! -x "$binary_path" ] || [ -n "$source_path" ] || { [ -f "$SCRIPT_DIR/.mise.toml" ] && [ "$SCRIPT_DIR/.mise.toml" -nt "$binary_path" ]; }; then
    echo "Building sand-config..." >&2
    if ! mise exec -C "$wizard_project" -- go build -o "$binary_path" .; then
      echo "Failed to build sand-config." >&2
      return 1
    fi
  fi

  printf '%s\n' "$binary_path"
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

  if [ ! -d "$workspace_input" ]; then
    echo "Workspace directory does not exist: $workspace_input" >&2
    return 1
  fi

  local workspace_dir manifest_path wizard_bin
  workspace_dir="$(cd "$workspace_input" && pwd -P)"
  manifest_path="$SCRIPT_DIR/updated/addons/manifest.tsv"

  if [ ! -f "$manifest_path" ]; then
    echo "Missing addons manifest: $manifest_path" >&2
    return 1
  fi

  wizard_bin="$(ensure_sand_config_binary)" || return 1

  exec "$wizard_bin" \
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
  local sand_config_bin=""

  sand_config_bin="$(ensure_sand_config_binary)" || exit 1

  parsed="$("$sand_config_bin" parse --path "$path")" || {
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
          workspace_mode) CONFIG_WORKSPACE_MODE="$value" ;;
          python_version) CONFIG_PYTHON_VERSION="$value" ;;
          uv_version) CONFIG_UV_VERSION="$value" ;;
          go_version) CONFIG_GO_VERSION="$value" ;;
          rust_version) CONFIG_RUST_VERSION="$value" ;;
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

  return 0
}

build_addon_build_arg() {
  local -n out_ref="$1"
  local addon
  local seen=","
  local joined=""
  out_ref=""

  for addon in "${CONFIG_ADDONS[@]}"; do
    if [[ ! "$addon" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
      warn "Invalid addon name in sand.toml skipped for build-time install: $addon"
      continue
    fi

    if [[ "$seen" == *",$addon,"* ]]; then
      continue
    fi

    seen="${seen}${addon},"

    if [ -n "$joined" ]; then
      joined="${joined},${addon}"
    else
      joined="$addon"
    fi
  done

  out_ref="$joined"
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

if [ "${1:-}" = "rebuild" ]; then
  shift

  # Parse rebuild-specific flags (only -d/--directory supported)
  rebuild_workspace_input=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -h|--help)
        cat >&2 <<'REBUILD_USAGE'
Usage: sand rebuild [-d <workspace_dir>]

Tears down and rebuilds the container for the current workspace and profile.
Settings (profile, mode, addons, etc.) are read from sand.toml.

Options:
  -d, --directory  workspace directory (default: current directory)
  -h, --help       show this help
REBUILD_USAGE
        exit 0
        ;;
      -d|--directory)
        if [ "$#" -lt 2 ]; then
          echo "Missing value for $1" >&2
          exit 1
        fi
        rebuild_workspace_input="$2"
        shift 2
        ;;
      -*)
        echo "Unknown option for sand rebuild: $1" >&2
        exit 1
        ;;
      *)
        if [ -z "$rebuild_workspace_input" ]; then
          rebuild_workspace_input="$1"
          shift
        else
          echo "Unexpected argument for sand rebuild: $1" >&2
          exit 1
        fi
        ;;
    esac
  done

  rebuild_workspace_input="${rebuild_workspace_input:-$PWD}"
  if [ ! -d "$rebuild_workspace_input" ]; then
    echo "Workspace directory does not exist: $rebuild_workspace_input" >&2
    exit 1
  fi

  REBUILD_WORKSPACE_DIR="$(cd "$rebuild_workspace_input" && pwd -P)"

  # Read sand.toml for profile
  rebuild_profile="0"
  if SAND_CONFIG_PATH="$(find_sand_toml "$REBUILD_WORKSPACE_DIR" 2>/dev/null)"; then
    parse_sand_toml "$SAND_CONFIG_PATH"
    if [ -n "$CONFIG_PROFILE" ]; then
      rebuild_profile="$(normalize_profile "$CONFIG_PROFILE")" || {
        echo "Invalid profile in sand.toml: '$CONFIG_PROFILE'" >&2
        exit 1
      }
    fi
  fi

  REBUILD_BASENAME="$(basename "$REBUILD_WORKSPACE_DIR")"
  REBUILD_SLUG="$(printf '%s' "$REBUILD_BASENAME" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-')"
  REBUILD_HASH="$(printf '%s' "$REBUILD_WORKSPACE_DIR" | sha1sum | awk '{print substr($1,1,8)}')"
  REBUILD_CONTAINER="sand-${REBUILD_SLUG}-${REBUILD_HASH}-${rebuild_profile}"

  if container_exists "$REBUILD_CONTAINER"; then
    echo "Tearing down container: $REBUILD_CONTAINER"
    docker rm -f "$REBUILD_CONTAINER" >/dev/null
  else
    echo "No existing container found: $REBUILD_CONTAINER (will build fresh)"
  fi

  echo "Rebuilding container..."
  exec "$0" -d "$REBUILD_WORKSPACE_DIR"
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
build_addons_arg=""

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

build_addon_build_arg build_addons_arg

# Resolve workspace_mode (mount or copy)
workspace_mode="${CONFIG_WORKSPACE_MODE:-mount}"
case "$workspace_mode" in
  mount|copy) ;;
  *)
    echo "Invalid workspace_mode in sand.toml: '$workspace_mode' (expected: mount|copy)" >&2
    exit 1
    ;;
esac
if [ "$mode" = "strict" ]; then
  if [ "$workspace_mode" = "mount" ] && [ -n "$CONFIG_WORKSPACE_MODE" ] && [ "$CONFIG_WORKSPACE_MODE" = "mount" ]; then
    warn "strict mode enforces workspace_mode=copy; overriding configured 'mount'."
  fi
  workspace_mode="copy"
fi

# Generate effective devcontainer config (disables bind mount in copy mode)
EFFECTIVE_CONFIG="$CONFIG_FILE"
COPY_MODE_TMP_DIR=""
if [ "$workspace_mode" = "copy" ]; then
  COPY_MODE_TMP_DIR="$(mktemp -d /tmp/sand-devcontainer-XXXXXX)"
  EFFECTIVE_CONFIG="$COPY_MODE_TMP_DIR/devcontainer.json"
  python3 -c '
import json, os, sys
original_path = sys.argv[1]
original_dir = os.path.dirname(os.path.abspath(original_path))
with open(original_path) as f:
    config = json.load(f)
config["workspaceMount"] = ""
config.setdefault("containerEnv", {})["SAND_WORKSPACE_MODE"] = "copy"
# Resolve relative dockerfile path so devcontainers CLI can find it from the temp dir
if "build" in config and "dockerfile" in config["build"]:
    df = config["build"]["dockerfile"]
    if not os.path.isabs(df):
        config["build"]["dockerfile"] = os.path.join(original_dir, df)
# Resolve context if present
if "build" in config and "context" in config["build"]:
    ctx = config["build"]["context"]
    if not os.path.isabs(ctx):
        config["build"]["context"] = os.path.join(original_dir, ctx)
elif "build" in config:
    config["build"]["context"] = original_dir
with open(sys.argv[2], "w") as f:
    json.dump(config, f, indent=2)
' "$CONFIG_FILE" "$EFFECTIVE_CONFIG"
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

  existing_ws_mode="$(container_env_value "$CONTAINER_NAME" "SAND_WORKSPACE_MODE" || true)"
  existing_ws_mode="${existing_ws_mode:-mount}"
  if [ "$workspace_mode" != "$existing_ws_mode" ]; then
    cat >&2 <<WARN_MSG
WARNING: Container '$CONTAINER_NAME' already exists with workspace_mode '$existing_ws_mode',
but you requested '$workspace_mode'. The existing workspace_mode is immutable and will be used.
To change workspace_mode, remove/recreate this workspace+profile container.
WARN_MSG
  fi
  workspace_mode="$existing_ws_mode"
else
  echo "Provisioning dev container via devcontainers CLI (profile=$profile mode=$mode workspace_mode=$workspace_mode)"
  if [ -n "$build_addons_arg" ]; then
    echo "Build-time addons requested from sand.toml: $build_addons_arg"
  fi
  (
    cd "$SCRIPT_DIR"
    SAND_PROFILE="$profile" \
      SAND_SECURITY_MODE="$mode" \
      SAND_WORKSPACE_MODE="$workspace_mode" \
      SAND_BUILD_MODE="$mode" \
      SAND_BUILD_ADDONS="$build_addons_arg" \
      SAND_PYTHON_VERSION="$CONFIG_PYTHON_VERSION" \
      SAND_UV_VERSION="$CONFIG_UV_VERSION" \
      SAND_GO_VERSION="$CONFIG_GO_VERSION" \
      SAND_RUST_VERSION="$CONFIG_RUST_VERSION" \
      SAND_DOTNET_VERSION="$CONFIG_DOTNET_VERSION" \
      SAND_JAVA_VERSION="$CONFIG_JAVA_VERSION" \
      SAND_MAVEN_VERSION="$CONFIG_MAVEN_VERSION" \
      SAND_GRADLE_VERSION="$CONFIG_GRADLE_VERSION" \
      SAND_BUN_VERSION="$CONFIG_BUN_VERSION" \
      SAND_DENO_VERSION="$CONFIG_DENO_VERSION" \
      npx @devcontainers/cli up \
        --workspace-folder "$WORKSPACE_DIR" \
        --config "$EFFECTIVE_CONFIG" \
        --id-label "devcontainer.local_folder=$WORKSPACE_DIR" \
        --id-label "devcontainer.config_file=$CONFIG_FILE" \
        --id-label "sand.profile=$profile"
  )

  # Clean up temp config dir now that provisioning is done
  if [ -n "$COPY_MODE_TMP_DIR" ] && [ -d "$COPY_MODE_TMP_DIR" ]; then
    rm -rf "$COPY_MODE_TMP_DIR"
  fi

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

  # In copy mode, copy the workspace into the container instead of bind-mounting
  if [ "$workspace_mode" = "copy" ]; then
    echo "Copying workspace into container (workspace_mode=copy)..."
    if ! container_running "$CONTAINER_NAME"; then
      docker start "$CONTAINER_NAME" >/dev/null
    fi
    docker cp "$WORKSPACE_DIR/." "$CONTAINER_NAME:/workspace/"
    docker exec --user root "$CONTAINER_NAME" chown -R node:node /workspace
    echo "Workspace copied. Host filesystem will not be modified."
  fi
fi

if ! container_running "$CONTAINER_NAME"; then
  docker start "$CONTAINER_NAME" >/dev/null
fi

apply_configured_addons "$CONTAINER_NAME" "$mode"

exec docker exec -it "$CONTAINER_NAME" zsh
