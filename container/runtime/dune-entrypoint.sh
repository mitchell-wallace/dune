#!/bin/sh
set -eu

warn() {
  printf '%s\n' "$*" >&2
}

ensure_timezone() {
  if [ -z "${TZ:-}" ]; then
    return 0
  fi

  if ! LC_ALL=C LANG=C LANGUAGE= sudo /usr/local/bin/dune-privileged ensure-timezone "$TZ"; then
    warn "WARNING: failed to ensure timezone '$TZ'"
  fi
}

ensure_locale() {
  requested_locale="${LC_ALL:-${LANG:-}}"
  if [ -z "$requested_locale" ]; then
    return 0
  fi

  if LC_ALL=C LANG=C LANGUAGE= sudo /usr/local/bin/dune-privileged ensure-locale "$requested_locale"; then
    return 0
  fi

  warn "WARNING: failed to ensure locale '$requested_locale'; falling back to en_AU.UTF-8"
  unset LC_ALL
  export LANG=en_AU.UTF-8
  export LANGUAGE="${LANGUAGE:-en_AU:en}"
}

ensure_timezone
ensure_locale

exec /usr/local/bin/docker-entrypoint.sh "$@"
