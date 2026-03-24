#!/usr/bin/env bash

normalize_locale_name() {
  local raw="${1:-}"
  local normalized
  normalized="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"
  normalized="${normalized/.utf-8/.utf8}"
  printf '%s\n' "$normalized"
}

locale_exists() {
  local requested="$1"
  local normalized_requested normalized_available
  normalized_requested="$(normalize_locale_name "$requested")"

  while IFS= read -r normalized_available; do
    if [ "$normalized_available" = "$normalized_requested" ]; then
      return 0
    fi
  done < <(locale -a 2>/dev/null | while IFS= read -r locale_name; do normalize_locale_name "$locale_name"; done)

  return 1
}

ensure_locale() {
  local requested locale_base charset normalized
  requested="${1:-${LC_ALL:-${LANG:-}}}"
  requested="$(printf '%s' "$requested" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"

  if [ -z "$requested" ]; then
    return 0
  fi

  requested="$(printf '%s' "$requested" | sed -E 's/\.utf8$/.UTF-8/I')"
  normalized="$(normalize_locale_name "$requested")"

  case "$normalized" in
    c|posix|c.utf8)
      return 0
      ;;
  esac

  if locale_exists "$requested"; then
    return 0
  fi

  if ! command -v localedef >/dev/null 2>&1; then
    echo "Unable to generate locale '$requested': localedef is not available" >&2
    return 1
  fi

  if [[ "$requested" == *.* ]]; then
    locale_base="${requested%%.*}"
    charset="${requested#*.}"
  else
    locale_base="$requested"
    charset="UTF-8"
    requested="${requested}.${charset}"
  fi

  case "$(printf '%s' "$charset" | tr '[:upper:]' '[:lower:]')" in
    utf8|utf-8)
      charset="UTF-8"
      ;;
  esac

  if ! localedef -i "$locale_base" -f "$charset" "$requested" >/dev/null 2>&1; then
    echo "Failed to generate locale '$requested' (source='$locale_base' charmap='$charset')" >&2
    return 1
  fi

  if ! locale_exists "$requested"; then
    echo "Locale '$requested' was generated but is still unavailable in locale -a" >&2
    return 1
  fi
}

ensure_timezone() {
  local requested zoneinfo
  requested="${1:-${TZ:-}}"
  requested="$(printf '%s' "$requested" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"

  if [ -z "$requested" ]; then
    return 0
  fi

  if [[ "$requested" == *".."* ]] || [[ "$requested" == /* ]]; then
    echo "Invalid timezone '$requested'" >&2
    return 1
  fi

  zoneinfo="/usr/share/zoneinfo/$requested"
  if [ ! -f "$zoneinfo" ]; then
    echo "Unknown timezone '$requested'" >&2
    return 1
  fi

  ln -snf "$zoneinfo" /etc/localtime
  printf '%s\n' "$requested" > /etc/timezone
}

get_effective_mode() {
  if [ -f "$MODE_FILE" ]; then
    cat "$MODE_FILE"
    return 0
  fi

  canonicalize_mode "${SAND_SECURITY_MODE:-std}"
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
