#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi
. "$UTILS_PATH"

GEAR_MANIFEST_PATH="/usr/local/lib/dune/gear/manifest.tsv"
GEAR_STATE_DIR="/persist/agent/gear"
LEGACY_ADDON_STATE_DIR="/persist/agent/addons"

get_mode() {
  if [ -f /etc/dune/security-mode ]; then
    canonicalize_mode "$(cat /etc/dune/security-mode)"
    return 0
  fi

  canonicalize_mode "${DUNE_SECURITY_MODE:-std}"
}

gear_state_path() {
  local gear_name="$1"
  printf '%s/%s.installed\n' "$GEAR_STATE_DIR" "$gear_name"
}

gear_installed() {
  local gear_name="$1"
  [ -f "$(gear_state_path "$gear_name")" ] || [ -f "${LEGACY_ADDON_STATE_DIR}/${gear_name}.installed" ]
}

print_gear_overview() {
  local mode name script description enabled_modes run_as helper_commands status
  local -a helper_lines helpers existing_helpers
  helper_lines=()

  mode="$(get_mode)"

  if [ "$mode" = "strict" ]; then
    echo "gear is disabled in strict mode"
    exit 1
  fi

  echo "gear: manage curated optional container capabilities."
  echo
  echo "Available gear for mode '$mode':"
  printf "  %-16s %-13s %s\n" "NAME" "STATUS" "DESCRIPTION"

  while IFS=$'\t' read -r name script description enabled_modes run_as helper_commands; do
    [ -z "$name" ] && continue
    [ "$name" = "name" ] && continue

    if ! mode_enabled "$mode" "$enabled_modes"; then
      continue
    fi

    helper_commands="${helper_commands:--}"
    status="not-installed"

    if gear_installed "$name"; then
      status="installed"

      if [ "$helper_commands" != "-" ] && [ -n "$helper_commands" ]; then
        IFS=',' read -r -a helpers <<<"$helper_commands"
        existing_helpers=()
        for helper in "${helpers[@]}"; do
          if command -v "$helper" >/dev/null 2>&1; then
            existing_helpers+=("$helper")
          fi
        done

        if [ "${#existing_helpers[@]}" -gt 0 ]; then
          helper_lines+=("  ${name}: ${existing_helpers[*]}")
        fi
      fi
    fi

    printf "  %-16s %-13s %s\n" "$name" "$status" "$description"
  done < "$GEAR_MANIFEST_PATH"

  echo
  echo "Available helper commands:"
  if [ "${#helper_lines[@]}" -eq 0 ]; then
    echo "  (none; install gear that provides helper commands)"
  else
    printf '%s\n' "${helper_lines[@]}"
  fi
}

install_gear() {
  local mode gear_name
  gear_name="$1"
  mode="$(get_mode)"

  if [ "$mode" = "strict" ]; then
    echo "gear is disabled in strict mode"
    exit 1
  fi

  sudo /usr/local/bin/dune-privileged run-gear "$gear_name"
}

cmd="${1:-list}"
case "$cmd" in
  list|help|-h|--help)
    print_gear_overview
    ;;
  status)
    print_gear_overview
    ;;
  install)
    if [ $# -lt 2 ]; then
      echo "Usage: gear install <name>" >&2
      exit 1
    fi
    install_gear "$2"
    ;;
  *)
    install_gear "$cmd"
    ;;
esac
