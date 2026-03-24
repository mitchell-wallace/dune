#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "dune-privileged must run as root" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UTILS_PATH="${DUNE_UTILS_PATH:-/usr/local/lib/dune/lib/utils.sh}"
MODULE_DIR="${DUNE_PRIVILEGED_MODULE_DIR:-/usr/local/lib/dune/runtime/dune-privileged}"

if [ ! -f "$UTILS_PATH" ]; then
  UTILS_PATH="${SCRIPT_DIR}/../lib/utils.sh"
fi

if [ ! -d "$MODULE_DIR" ]; then
  MODULE_DIR="${SCRIPT_DIR}/dune-privileged"
fi

# Load shared helpers and split command implementations from either the installed
# image layout or the repository checkout.
. "$UTILS_PATH"
. "${MODULE_DIR}/config.sh"
. "${MODULE_DIR}/services.sh"
. "${MODULE_DIR}/gear.sh"

GEAR_DIR="/usr/local/lib/dune/gear"
GEAR_MANIFEST_PATH="${GEAR_DIR}/manifest.tsv"
GEAR_STATE_DIR="/persist/agent/gear"
DUNE_ETC_DIR="/etc/dune"
MODE_FILE="${DUNE_ETC_DIR}/security-mode"
PROFILE_FILE="${DUNE_ETC_DIR}/profile"
NODE_LAX_SUDOERS="/etc/sudoers.d/node-lax"

cmd="${1:-}"
case "$cmd" in
  init-firewall)
    exec /usr/local/bin/init-firewall.sh
    ;;
  configure-mode)
    configure_mode "${2:-}" "${3:-}"
    ;;
  run-gear)
    run_gear "${2:-}"
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
    echo "Usage: dune-privileged <init-firewall|configure-mode|run-gear|pg-local|redis-local|mp-local|ensure-locale|ensure-timezone>" >&2
    exit 1
    ;;
esac
