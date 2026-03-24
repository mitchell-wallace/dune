#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi
. "$UTILS_PATH"

autostart_gear_service() {
  local gear_name="$1"
  local service_cmd="$2"
  local label="$3"
  local helper_cmd="$4"
  local state_file="/persist/agent/gear/${gear_name}.installed"

  if [ ! -f "$state_file" ]; then
    return 0
  fi

  if ! command -v "$helper_cmd" >/dev/null 2>&1; then
    echo "Reinstalling persisted gear '${gear_name}'"
    if ! sudo -n /usr/local/bin/dune-privileged run-gear "$gear_name"; then
      echo "WARNING: failed to reinstall persisted gear '${gear_name}'" >&2
      return 1
    fi
  fi

  if ! sudo -n /usr/local/bin/dune-privileged "$service_cmd" start; then
    echo "WARNING: failed to autostart ${label} for installed gear '${gear_name}'" >&2
    return 1
  fi

  return 0
}

autostart_installed_services() {
  local failed=0

  if [ "$MODE" = "strict" ]; then
    return 0
  fi

  autostart_gear_service "add-postgres" "pg-local" "PostgreSQL" "pg-local" || failed=1
  autostart_gear_service "add-redis" "redis-local" "Redis" "redis-local" || failed=1
  autostart_gear_service "add-mailpit" "mp-local" "Mailpit" "mp-local" || failed=1

  return "$failed"
}

MODE="$(canonicalize_mode "${DUNE_SECURITY_MODE:-std}")" || {
  echo "Invalid DUNE_SECURITY_MODE='${DUNE_SECURITY_MODE:-}' in container env" >&2
  exit 1
}

PROFILE="$(normalize_profile "${DUNE_PROFILE:-0}")" || {
  echo "Invalid DUNE_PROFILE='${DUNE_PROFILE:-}' in container env" >&2
  exit 1
}

REQUESTED_LOCALE="${LC_ALL:-${LANG:-}}"
if [ -n "$REQUESTED_LOCALE" ]; then
  if ! sudo /usr/local/bin/dune-privileged ensure-locale "$REQUESTED_LOCALE"; then
    echo "WARNING: failed to ensure locale '$REQUESTED_LOCALE'" >&2
  fi
fi

/usr/local/bin/setup-agent-persist.sh
sudo /usr/local/bin/dune-privileged configure-mode "$MODE" "$PROFILE"

if [ "$MODE" = "yolo" ]; then
  echo "Skipping firewall setup in yolo mode"
else
  sudo /usr/local/bin/dune-privileged init-firewall
fi

autostart_installed_services
