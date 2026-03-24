#!/usr/bin/env bash

lookup_gear() {
  local gear_name="$1"

  awk -F'\t' -v gear_name="$gear_name" '
    NR == 1 { next }
    NF < 5 { next }
    $1 == gear_name { print; found=1; exit }
    END { if (!found) exit 1 }
  ' "$GEAR_MANIFEST_PATH"
}

gear_state_path() {
  local gear_name="$1"
  printf '%s/%s.installed\n' "$GEAR_STATE_DIR" "$gear_name"
}

validate_helper_commands() {
  local helper_commands="$1"
  local helper

  if [ "$helper_commands" = "-" ] || [ -z "$helper_commands" ]; then
    return 0
  fi

  IFS=',' read -r -a helpers <<<"$helper_commands"
  for helper in "${helpers[@]}"; do
    if [[ ! "$helper" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
      echo "Invalid helper command name in manifest: $helper" >&2
      exit 1
    fi
  done
}

mark_gear_installed() {
  local gear_name="$1"
  local helper_commands="$2"
  local state_file
  state_file="$(gear_state_path "$gear_name")"

  mkdir -p "$GEAR_STATE_DIR"
  chmod 0755 "$GEAR_STATE_DIR"

  {
    printf 'installed_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'helper_commands=%s\n' "$helper_commands"
  } > "$state_file"
  chmod 0644 "$state_file"
}

run_gear() {
  local gear_name row name script description enabled_modes run_as helper_commands script_path mode rc
  gear_name="${1:-}"

  if [ -z "$gear_name" ]; then
    echo "Usage: dune-privileged run-gear <name>" >&2
    exit 1
  fi

  if [[ ! "$gear_name" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
    echo "Invalid gear name: $gear_name" >&2
    exit 1
  fi

  mode="$(get_effective_mode)"
  mode="$(canonicalize_mode "$mode")"

  if [ "$mode" = "strict" ]; then
    echo "gear is disabled in strict mode" >&2
    exit 1
  fi

  row="$(lookup_gear "$gear_name")" || {
    echo "Unknown gear: $gear_name" >&2
    exit 1
  }

  IFS=$'\t' read -r name script description enabled_modes run_as helper_commands <<<"$row"
  helper_commands="${helper_commands:--}"

  if [ "$name" != "$gear_name" ]; then
    echo "Gear lookup mismatch for $gear_name" >&2
    exit 1
  fi

  if [[ "$script" = */* ]]; then
    echo "Invalid manifest script path for $gear_name" >&2
    exit 1
  fi

  validate_helper_commands "$helper_commands"

  if ! mode_enabled "$mode" "$enabled_modes"; then
    echo "Gear '$gear_name' is not enabled in mode '$mode'" >&2
    exit 1
  fi

  script_path="${GEAR_DIR}/${script}"
  if [ ! -f "$script_path" ]; then
    echo "Gear script missing: $script_path" >&2
    exit 1
  fi

  set +e
  case "$run_as" in
    root)
      env \
        HOME="/home/node" \
        USER="node" \
        LOGNAME="node" \
        DUNE_TARGET_HOME="/home/node" \
        DUNE_TARGET_USER="node" \
        DUNE_SECURITY_MODE="$mode" \
        "$script_path"
      rc=$?
      ;;
    node)
      su - node -c "DUNE_SECURITY_MODE='$mode' '$script_path'"
      rc=$?
      ;;
    *)
      set -e
      echo "Invalid run_as in manifest for $gear_name: $run_as" >&2
      exit 1
      ;;
  esac
  set -e

  if [ "$rc" -eq 0 ]; then
    mark_gear_installed "$gear_name" "$helper_commands"
  fi

  exit "$rc"
}
