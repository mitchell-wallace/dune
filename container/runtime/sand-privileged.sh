#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "sand-privileged must run as root" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${SAND_UTILS_PATH:-/usr/local/lib/sand/lib/utils.sh}"
MODULE_DIR="${SAND_PRIVILEGED_MODULE_DIR:-/usr/local/lib/sand/runtime/sand-privileged}"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi

if [ ! -d "$MODULE_DIR" ]; then
  MODULE_DIR="${SCRIPT_DIR}/sand-privileged"
fi

# Load shared helpers and split command implementations from either the installed
# image layout or the repository checkout.
. "$UTILS_PATH"
. "${MODULE_DIR}/config.sh"
. "${MODULE_DIR}/services.sh"
. "${MODULE_DIR}/addons.sh"

ADDON_DIR="/usr/local/lib/sand/addons"
MANIFEST_PATH="${ADDON_DIR}/manifest.tsv"
ADDON_STATE_DIR="/persist/agent/addons"
SAND_ETC_DIR="/etc/sand"
MODE_FILE="${SAND_ETC_DIR}/security-mode"
PROFILE_FILE="${SAND_ETC_DIR}/profile"
NODE_LAX_SUDOERS="/etc/sudoers.d/node-lax"

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
  pg-local)
    pg_local_cmd "${2:-help}"
    ;;
  redis-local)
    redis_local_cmd "${2:-help}"
    ;;
  mp-local)
    mp_local_cmd "${2:-help}"
    ;;
  ensure-locale)
    ensure_locale "${2:-}"
    ;;
  ensure-timezone)
    ensure_timezone "${2:-}"
    ;;
  *)
    echo "Usage: sand-privileged <init-firewall|configure-mode|run-addon|pg-local|redis-local|mp-local|ensure-locale|ensure-timezone>" >&2
    exit 1
    ;;
esac
