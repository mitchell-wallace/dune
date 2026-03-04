#!/usr/bin/env bash
set -euo pipefail

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

MODE="$(canonicalize_mode "${SAND_SECURITY_MODE:-std}")" || {
  echo "Invalid SAND_SECURITY_MODE='${SAND_SECURITY_MODE:-}' in container env" >&2
  exit 1
}

PROFILE="$(normalize_profile "${SAND_PROFILE:-0}")" || {
  echo "Invalid SAND_PROFILE='${SAND_PROFILE:-}' in container env" >&2
  exit 1
}

REQUESTED_LOCALE="${LC_ALL:-${LANG:-}}"
if [ -n "$REQUESTED_LOCALE" ]; then
  if ! sudo /usr/local/bin/sand-privileged ensure-locale "$REQUESTED_LOCALE"; then
    echo "WARNING: failed to ensure locale '$REQUESTED_LOCALE'" >&2
  fi
fi

/usr/local/bin/setup-agent-persist.sh
sudo /usr/local/bin/sand-privileged configure-mode "$MODE" "$PROFILE"

if [ "$MODE" = "yolo" ]; then
  echo "Skipping firewall setup in yolo mode"
  exit 0
fi

sudo /usr/local/bin/sand-privileged init-firewall
