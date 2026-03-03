#!/usr/bin/env bash
set -euo pipefail

MANIFEST_PATH="/usr/local/lib/sand/addons/manifest.tsv"
ADDON_STATE_DIR="/persist/agent/addons"

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

get_mode() {
  if [ -f /etc/sand/security-mode ]; then
    canonicalize_mode "$(cat /etc/sand/security-mode)"
    return 0
  fi

  canonicalize_mode "${SAND_SECURITY_MODE:-std}"
}

addon_state_path() {
  local addon_name="$1"
  printf '%s/%s.installed\n' "$ADDON_STATE_DIR" "$addon_name"
}

addon_installed() {
  local addon_name="$1"
  [ -f "$(addon_state_path "$addon_name")" ]
}

print_addons_overview() {
  local mode name script description enabled_modes run_as helper_commands status
  local -a helper_lines helpers existing_helpers
  helper_lines=()

  mode="$(get_mode)"

  if [ "$mode" = "strict" ]; then
    echo "addons are disabled in strict mode"
    exit 1
  fi

  echo "addons: run curated optional extras (no arbitrary scripts)."
  echo
  echo "Available addons for mode '$mode':"
  printf "  %-16s %-13s %s\n" "NAME" "STATUS" "DESCRIPTION"

  while IFS=$'\t' read -r name script description enabled_modes run_as helper_commands; do
    [ -z "$name" ] && continue
    [ "$name" = "name" ] && continue

    if ! mode_enabled "$mode" "$enabled_modes"; then
      continue
    fi

    helper_commands="${helper_commands:--}"
    status="not-installed"

    if addon_installed "$name"; then
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
  done < "$MANIFEST_PATH"

  echo
  echo "Available helper commands:"
  if [ "${#helper_lines[@]}" -eq 0 ]; then
    echo "  (none; install addons that provide helper commands)"
  else
    printf '%s\n' "${helper_lines[@]}"
  fi
}

run_addon() {
  local mode addon_name
  addon_name="$1"
  mode="$(get_mode)"

  if [ "$mode" = "strict" ]; then
    echo "addons are disabled in strict mode"
    exit 1
  fi

  sudo /usr/local/bin/sand-privileged run-addon "$addon_name"
}

cmd="${1:-list}"
case "$cmd" in
  list|help|-h|--help)
    print_addons_overview
    ;;
  *)
    run_addon "$cmd"
    ;;
esac
