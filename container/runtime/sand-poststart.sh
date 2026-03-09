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

autostart_addon_service() {
  local addon_name="$1"
  local service_cmd="$2"
  local label="$3"
  local helper_cmd="$4"
  local state_file="/persist/agent/addons/${addon_name}.installed"

  if [ ! -f "$state_file" ]; then
    return 0
  fi

  if ! command -v "$helper_cmd" >/dev/null 2>&1; then
    echo "Reinstalling persisted addon '${addon_name}'"
    if ! sudo -n /usr/local/bin/sand-privileged run-addon "$addon_name"; then
      echo "WARNING: failed to reinstall persisted addon '${addon_name}'" >&2
      return 1
    fi
  fi

  if ! sudo -n /usr/local/bin/sand-privileged "$service_cmd" start; then
    echo "WARNING: failed to autostart ${label} for installed addon '${addon_name}'" >&2
    return 1
  fi

  return 0
}

autostart_installed_services() {
  local failed=0

  if [ "$MODE" = "strict" ]; then
    return 0
  fi

  autostart_addon_service "add-postgres" "pg-local" "PostgreSQL" "pg-local" || failed=1
  autostart_addon_service "add-redis" "redis-local" "Redis" "redis-local" || failed=1
  autostart_addon_service "add-mailpit" "mp-local" "Mailpit" "mp-local" || failed=1

  return "$failed"
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
else
  sudo /usr/local/bin/sand-privileged init-firewall
fi

autostart_installed_services
