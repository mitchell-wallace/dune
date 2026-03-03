#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "sand-privileged must run as root" >&2
  exit 1
fi

ADDON_DIR="/usr/local/lib/sand/addons"
MANIFEST_PATH="${ADDON_DIR}/manifest.tsv"
SAND_ETC_DIR="/etc/sand"
MODE_FILE="${SAND_ETC_DIR}/security-mode"
PROFILE_FILE="${SAND_ETC_DIR}/profile"
NODE_LAX_SUDOERS="/etc/sudoers.d/node-lax"

canonicalize_mode() {
  local raw="${1:-std}"
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
  local raw="${1:-0}"
  local profile
  profile="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  if [[ "$profile" =~ ^[0-9a-z]$ ]]; then
    printf '%s\n' "$profile"
    return 0
  fi

  return 1
}

get_effective_mode() {
  if [ -f "$MODE_FILE" ]; then
    cat "$MODE_FILE"
    return 0
  fi

  canonicalize_mode "${SAND_SECURITY_MODE:-std}"
}

mode_enabled() {
  local mode="$1"
  local mode_list="$2"

  IFS=',' read -r -a items <<<"$mode_list"
  for item in "${items[@]}"; do
    if [ "$item" = "$mode" ]; then
      return 0
    fi
  done

  return 1
}

lookup_addon() {
  local addon_name="$1"

  awk -F'\t' -v addon_name="$addon_name" '
    NR == 1 { next }
    NF < 5 { next }
    $1 == addon_name { print; found=1; exit }
    END { if (!found) exit 1 }
  ' "$MANIFEST_PATH"
}

configure_mode() {
  local requested_mode requested_profile
  requested_mode="$(canonicalize_mode "${1:-${SAND_SECURITY_MODE:-std}}")" || {
    echo "Invalid security mode: ${1:-${SAND_SECURITY_MODE:-std}}" >&2
    exit 1
  }

  requested_profile="$(normalize_profile "${2:-${SAND_PROFILE:-0}}")" || {
    echo "Invalid profile: ${2:-${SAND_PROFILE:-0}}" >&2
    exit 1
  }

  mkdir -p "$SAND_ETC_DIR"
  chmod 0755 "$SAND_ETC_DIR"

  if [ -f "$MODE_FILE" ]; then
    return 0
  fi

  printf '%s\n' "$requested_mode" > "$MODE_FILE"
  chmod 0644 "$MODE_FILE"

  printf '%s\n' "$requested_profile" > "$PROFILE_FILE"
  chmod 0644 "$PROFILE_FILE"

  if [ "$requested_mode" = "lax" ] || [ "$requested_mode" = "yolo" ]; then
    echo "node ALL=(ALL) NOPASSWD: ALL" > "$NODE_LAX_SUDOERS"
    chmod 0440 "$NODE_LAX_SUDOERS"
  else
    rm -f "$NODE_LAX_SUDOERS"
  fi
}

run_addon() {
  local addon_name row name script description enabled_modes run_as script_path mode
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

  IFS=$'\t' read -r name script description enabled_modes run_as <<<"$row"

  if [ "$name" != "$addon_name" ]; then
    echo "Addon lookup mismatch for $addon_name" >&2
    exit 1
  fi

  if [[ "$script" = */* ]]; then
    echo "Invalid manifest script path for $addon_name" >&2
    exit 1
  fi

  if ! mode_enabled "$mode" "$enabled_modes"; then
    echo "Addon '$addon_name' is not enabled in mode '$mode'" >&2
    exit 1
  fi

  script_path="${ADDON_DIR}/${script}"
  if [ ! -f "$script_path" ]; then
    echo "Addon script missing: $script_path" >&2
    exit 1
  fi

  case "$run_as" in
    root)
      exec env \
        HOME="/home/node" \
        USER="node" \
        LOGNAME="node" \
        SAND_TARGET_HOME="/home/node" \
        SAND_TARGET_USER="node" \
        SAND_SECURITY_MODE="$mode" \
        "$script_path"
      ;;
    node)
      exec su - node -c "SAND_SECURITY_MODE='$mode' '$script_path'"
      ;;
    *)
      echo "Invalid run_as in manifest for $addon_name: $run_as" >&2
      exit 1
      ;;
  esac
}

cmd="${1:-}"
case "$cmd" in
  init-firewall)
    exec /usr/local/bin/init-firewall.sh
    ;;
  configure-mode)
    configure_mode "${2:-}" "${3:-}"
    ;;
  run-addon)
    run_addon "${2:-}"
    ;;
  *)
    echo "Usage: sand-privileged <init-firewall|configure-mode|run-addon>" >&2
    exit 1
    ;;
esac
