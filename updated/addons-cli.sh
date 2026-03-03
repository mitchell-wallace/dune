#!/usr/bin/env bash
set -euo pipefail

MANIFEST_PATH="/usr/local/lib/sand/addons/manifest.tsv"

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

list_addons() {
  local mode
  mode="$(get_mode)"

  if [ "$mode" = "strict" ]; then
    echo "addons are disabled in strict mode"
    exit 1
  fi

  echo "Enabled addons for mode '$mode':"
  awk -F'\t' -v mode="$mode" '
    NR == 1 { next }
    NF < 5 { next }
    {
      split($4, modes, ",")
      for (i in modes) {
        if (modes[i] == mode) {
          printf "  %-16s %s\n", $1, $3
          break
        }
      }
    }
  ' "$MANIFEST_PATH"
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
  list)
    list_addons
    ;;
  -h|--help|help)
    cat <<'EOF_HELP'
Usage:
  addons list
  addons <addon-name>

Examples:
  addons
  addons add-omc
  addons boost-cli
EOF_HELP
    ;;
  *)
    run_addon "$cmd"
    ;;
esac
