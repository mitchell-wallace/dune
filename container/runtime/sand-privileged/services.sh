#!/usr/bin/env bash

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
