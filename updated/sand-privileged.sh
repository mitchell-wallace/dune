#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "sand-privileged must run as root" >&2
  exit 1
fi

ADDON_DIR="/usr/local/lib/sand/addons"
MANIFEST_PATH="${ADDON_DIR}/manifest.tsv"
ADDON_STATE_DIR="/persist/agent/addons"
SAND_ETC_DIR="/etc/sand"
MODE_FILE="${SAND_ETC_DIR}/security-mode"
PROFILE_FILE="${SAND_ETC_DIR}/profile"
NODE_LAX_SUDOERS="/etc/sudoers.d/node-lax"

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

lookup_addon() {
  local addon_name="$1"

  awk -F'\t' -v addon_name="$addon_name" '
    NR == 1 { next }
    NF < 5 { next }
    $1 == addon_name { print; found=1; exit }
    END { if (!found) exit 1 }
  ' "$MANIFEST_PATH"
}

addon_state_path() {
  local addon_name="$1"
  printf '%s/%s.installed\n' "$ADDON_STATE_DIR" "$addon_name"
}

validate_helper_commands() {
  local helper_commands="$1"
  local helper

  if [ "$helper_commands" = "-" ] || [ -z "$helper_commands" ]; then
    return 0
  fi

  IFS=',' read -r -a helpers <<<"$helper_commands"
  for helper in "${helpers[@]}"; do
    if [[ ! "$helper" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
      echo "Invalid helper command name in manifest: $helper" >&2
      exit 1
    fi
  done
}

mark_addon_installed() {
  local addon_name="$1"
  local helper_commands="$2"
  local state_file
  state_file="$(addon_state_path "$addon_name")"

  mkdir -p "$ADDON_STATE_DIR"
  chmod 0755 "$ADDON_STATE_DIR"

  {
    printf 'installed_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'helper_commands=%s\n' "$helper_commands"
  } > "$state_file"
  chmod 0644 "$state_file"
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

get_pg_version() {
  local cluster_dir
  cluster_dir="$(find /etc/postgresql -mindepth 2 -maxdepth 2 -type d -name main 2>/dev/null | sort -V | tail -n1 || true)"
  if [ -z "$cluster_dir" ]; then
    return 1
  fi
  basename "$(dirname "$cluster_dir")"
}

pg_local_usage() {
  cat <<'EOF_PG_USAGE'
Usage: pg-local <start|stop|restart|status|logs|shell|url>
EOF_PG_USAGE
}

pg_local_cmd() {
  local cmd pg_version log_file
  cmd="${1:-help}"

  pg_version="$(get_pg_version)" || {
    echo "PostgreSQL is not installed. Run: addons add-postgres" >&2
    exit 1
  }

  case "$cmd" in
    start)
      if ! pg_ctlcluster "$pg_version" main status >/dev/null 2>&1; then
        pg_ctlcluster "$pg_version" main start >/dev/null 2>&1 || true
      fi
      if ! pg_ctlcluster "$pg_version" main status >/dev/null 2>&1; then
        echo "Failed to start postgresql (${pg_version})" >&2
        exit 1
      fi
      echo "postgresql (${pg_version}) started"
      ;;
    stop)
      pg_ctlcluster "$pg_version" main stop >/dev/null 2>&1 || true
      echo "postgresql (${pg_version}) stopped"
      ;;
    restart)
      pg_ctlcluster "$pg_version" main restart >/dev/null 2>&1
      echo "postgresql (${pg_version}) restarted"
      ;;
    status)
      if pg_ctlcluster "$pg_version" main status >/dev/null 2>&1; then
        echo "postgresql (${pg_version}) is running"
      else
        echo "postgresql (${pg_version}) is stopped"
        exit 1
      fi
      ;;
    logs)
      log_file="/var/log/postgresql/postgresql-${pg_version}-main.log"
      if [ -f "$log_file" ]; then
        tail -n 50 "$log_file"
      else
        echo "No PostgreSQL log found at $log_file"
      fi
      ;;
    shell)
      exec runuser -u node -- env \
        PGHOST=127.0.0.1 \
        PGPORT=5432 \
        PGUSER=node \
        PGDATABASE=app \
        psql
      ;;
    url)
      echo "postgresql://node@127.0.0.1:5432/app"
      ;;
    help|-h|--help)
      pg_local_usage
      ;;
    *)
      pg_local_usage >&2
      exit 1
      ;;
  esac
}

redis_local_usage() {
  cat <<'EOF_REDIS_USAGE'
Usage: redis-local <start|stop|restart|status|logs|shell|url>
EOF_REDIS_USAGE
}

redis_local_running() {
  redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1
}

redis_local_cmd() {
  local cmd redis_conf redis_log redis_pid
  cmd="${1:-help}"
  redis_conf="/etc/redis/redis-local.conf"
  redis_log="/var/log/redis/redis-local.log"
  redis_pid="/var/run/redis-local.pid"

  if ! command -v redis-server >/dev/null 2>&1; then
    echo "Redis is not installed. Run: addons add-redis" >&2
    exit 1
  fi

  case "$cmd" in
    start)
      if redis_local_running; then
        echo "redis is already running"
      else
        redis-server "$redis_conf"
        echo "redis started"
      fi
      ;;
    stop)
      if redis_local_running; then
        redis-cli -h 127.0.0.1 -p 6379 shutdown nosave >/dev/null 2>&1 || true
      fi
      if [ -f "$redis_pid" ]; then
        kill "$(cat "$redis_pid")" >/dev/null 2>&1 || true
        rm -f "$redis_pid"
      fi
      echo "redis stopped"
      ;;
    restart)
      "$0" redis-local stop >/dev/null 2>&1 || true
      "$0" redis-local start
      ;;
    status)
      if redis_local_running; then
        echo "redis is running"
      else
        echo "redis is stopped"
        exit 1
      fi
      ;;
    logs)
      if [ -f "$redis_log" ]; then
        tail -n 50 "$redis_log"
      else
        echo "No Redis log found at $redis_log"
      fi
      ;;
    shell)
      exec runuser -u node -- redis-cli -h 127.0.0.1 -p 6379
      ;;
    url)
      echo "redis://127.0.0.1:6379"
      ;;
    help|-h|--help)
      redis_local_usage
      ;;
    *)
      redis_local_usage >&2
      exit 1
      ;;
  esac
}

mp_local_usage() {
  cat <<'EOF_MAILPIT_USAGE'
Usage: mp-local <start|stop|restart|status|logs|url>
EOF_MAILPIT_USAGE
}

mp_local_running() {
  local pid_file pid
  pid_file="/var/run/mailpit-local.pid"

  if [ ! -f "$pid_file" ]; then
    return 1
  fi

  pid="$(cat "$pid_file" 2>/dev/null || true)"
  if [[ ! "$pid" =~ ^[0-9]+$ ]]; then
    rm -f "$pid_file"
    return 1
  fi

  if kill -0 "$pid" >/dev/null 2>&1; then
    return 0
  fi

  rm -f "$pid_file"
  return 1
}

mp_local_cmd() {
  local cmd log_dir log_file pid_file ui_addr smtp_addr pid
  cmd="${1:-help}"
  log_dir="/var/log/mailpit"
  log_file="${log_dir}/mailpit-local.log"
  pid_file="/var/run/mailpit-local.pid"
  ui_addr="127.0.0.1:8025"
  smtp_addr="127.0.0.1:1025"

  if ! command -v mailpit >/dev/null 2>&1; then
    echo "Mailpit is not installed. Run: addons add-mailpit" >&2
    exit 1
  fi

  case "$cmd" in
    start)
      if mp_local_running; then
        echo "mailpit is already running"
      else
        mkdir -p "$log_dir"
        touch "$log_file"
        chown node:node "$log_dir" "$log_file" >/dev/null 2>&1 || true
        nohup mailpit --listen "$ui_addr" --smtp "$smtp_addr" >>"$log_file" 2>&1 &
        pid=$!
        echo "$pid" > "$pid_file"
        chmod 0644 "$pid_file"
        sleep 1
        if kill -0 "$pid" >/dev/null 2>&1; then
          echo "mailpit started (ui=http://${ui_addr} smtp=${smtp_addr})"
        else
          rm -f "$pid_file"
          echo "Failed to start mailpit. Check logs with: mp-local logs" >&2
          exit 1
        fi
      fi
      ;;
    stop)
      if mp_local_running; then
        pid="$(cat "$pid_file")"
        kill "$pid" >/dev/null 2>&1 || true
        for _ in $(seq 1 10); do
          if kill -0 "$pid" >/dev/null 2>&1; then
            sleep 0.2
          else
            break
          fi
        done
        if kill -0 "$pid" >/dev/null 2>&1; then
          kill -9 "$pid" >/dev/null 2>&1 || true
        fi
        rm -f "$pid_file"
      fi
      echo "mailpit stopped"
      ;;
    restart)
      "$0" mp-local stop >/dev/null 2>&1 || true
      "$0" mp-local start
      ;;
    status)
      if mp_local_running; then
        echo "mailpit is running"
      else
        echo "mailpit is stopped"
        exit 1
      fi
      ;;
    logs)
      if [ -f "$log_file" ]; then
        tail -n 50 "$log_file"
      else
        echo "No Mailpit log found at $log_file"
      fi
      ;;
    url)
      echo "http://127.0.0.1:8025"
      echo "smtp://127.0.0.1:1025"
      ;;
    help|-h|--help)
      mp_local_usage
      ;;
    *)
      mp_local_usage >&2
      exit 1
      ;;
  esac
}

run_addon() {
  local addon_name row name script description enabled_modes run_as helper_commands script_path mode rc
  addon_name="${1:-}"

  if [ -z "$addon_name" ]; then
    echo "Usage: sand-privileged run-addon <addon-name>" >&2
    exit 1
  fi

  if [[ ! "$addon_name" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
    echo "Invalid addon name: $addon_name" >&2
    exit 1
  fi

  mode="$(get_effective_mode)"
  mode="$(canonicalize_mode "$mode")"

  if [ "$mode" = "strict" ]; then
    echo "addons are disabled in strict mode" >&2
    exit 1
  fi

  row="$(lookup_addon "$addon_name")" || {
    echo "Unknown addon: $addon_name" >&2
    exit 1
  }

  IFS=$'\t' read -r name script description enabled_modes run_as helper_commands <<<"$row"
  helper_commands="${helper_commands:--}"

  if [ "$name" != "$addon_name" ]; then
    echo "Addon lookup mismatch for $addon_name" >&2
    exit 1
  fi

  if [[ "$script" = */* ]]; then
    echo "Invalid manifest script path for $addon_name" >&2
    exit 1
  fi

  validate_helper_commands "$helper_commands"

  if ! mode_enabled "$mode" "$enabled_modes"; then
    echo "Addon '$addon_name' is not enabled in mode '$mode'" >&2
    exit 1
  fi

  script_path="${ADDON_DIR}/${script}"
  if [ ! -f "$script_path" ]; then
    echo "Addon script missing: $script_path" >&2
    exit 1
  fi

  set +e
  case "$run_as" in
    root)
      env \
        HOME="/home/node" \
        USER="node" \
        LOGNAME="node" \
        SAND_TARGET_HOME="/home/node" \
        SAND_TARGET_USER="node" \
        SAND_SECURITY_MODE="$mode" \
        "$script_path"
      rc=$?
      ;;
    node)
      su - node -c "SAND_SECURITY_MODE='$mode' '$script_path'"
      rc=$?
      ;;
    *)
      set -e
      echo "Invalid run_as in manifest for $addon_name: $run_as" >&2
      exit 1
      ;;
  esac
  set -e

  if [ "$rc" -eq 0 ]; then
    mark_addon_installed "$addon_name" "$helper_commands"
  fi

  exit "$rc"
}

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
