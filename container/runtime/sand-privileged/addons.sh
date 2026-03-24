#!/usr/bin/env bash

lookup_addon() {
  local addon_name="$1"

  awk -F'\t' -v addon_name="$addon_name" '
    NR == 1 { next }
    NF < 5 { next }
    $1 == addon_name { print; found=1; exit }
    END { if (!found) exit 1 }
  ' "$MANIFEST_PATH"
}

addon_state_path() {
  local addon_name="$1"
  printf '%s/%s.installed\n' "$ADDON_STATE_DIR" "$addon_name"
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

mark_addon_installed() {
  local addon_name="$1"
  local helper_commands="$2"
  local state_file
  state_file="$(addon_state_path "$addon_name")"

  mkdir -p "$ADDON_STATE_DIR"
  chmod 0755 "$ADDON_STATE_DIR"

  {
    printf 'installed_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'helper_commands=%s\n' "$helper_commands"
  } > "$state_file"
  chmod 0644 "$state_file"
}

run_addon() {
  local addon_name row name script description enabled_modes run_as helper_commands script_path mode rc
  addon_name="${1:-}"

  if [ -z "$addon_name" ]; then
    echo "Usage: sand-privileged run-addon <addon-name>" >&2
    exit 1
  fi

  if [[ ! "$addon_name" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
    echo "Invalid addon name: $addon_name" >&2
    exit 1
  fi

  mode="$(get_effective_mode)"
  mode="$(canonicalize_mode "$mode")"

  if [ "$mode" = "strict" ]; then
    echo "addons are disabled in strict mode" >&2
    exit 1
  fi

  row="$(lookup_addon "$addon_name")" || {
    echo "Unknown addon: $addon_name" >&2
    exit 1
  }

  IFS=$'\t' read -r name script description enabled_modes run_as helper_commands <<<"$row"
  helper_commands="${helper_commands:--}"

  if [ "$name" != "$addon_name" ]; then
    echo "Addon lookup mismatch for $addon_name" >&2
    exit 1
  fi

  if [[ "$script" = */* ]]; then
    echo "Invalid manifest script path for $addon_name" >&2
    exit 1
  fi

  validate_helper_commands "$helper_commands"

  if ! mode_enabled "$mode" "$enabled_modes"; then
    echo "Addon '$addon_name' is not enabled in mode '$mode'" >&2
    exit 1
  fi

  script_path="${ADDON_DIR}/${script}"
  if [ ! -f "$script_path" ]; then
    echo "Addon script missing: $script_path" >&2
    exit 1
  fi

  set +e
  case "$run_as" in
    root)
      env \
        HOME="/home/node" \
        USER="node" \
        LOGNAME="node" \
        SAND_TARGET_HOME="/home/node" \
        SAND_TARGET_USER="node" \
        SAND_SECURITY_MODE="$mode" \
        "$script_path"
      rc=$?
      ;;
    node)
      su - node -c "SAND_SECURITY_MODE='$mode' '$script_path'"
      rc=$?
      ;;
    *)
      set -e
      echo "Invalid run_as in manifest for $addon_name: $run_as" >&2
      exit 1
      ;;
  esac
  set -e

  if [ "$rc" -eq 0 ]; then
    mark_addon_installed "$addon_name" "$helper_commands"
  fi

  exit "$rc"
}
