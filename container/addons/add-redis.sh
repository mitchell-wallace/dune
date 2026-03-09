#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-redis] must run as root" >&2
  exit 1
fi

log() {
  echo "[add-redis] $*"
}

install_helper() {
  cat > /usr/local/bin/redis-local <<'EOF_HELPER'
#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -eq 0 ]; then
  exec /usr/local/bin/sand-privileged redis-local "$@"
fi

exec sudo /usr/local/bin/sand-privileged redis-local "$@"
EOF_HELPER
  chmod 0755 /usr/local/bin/redis-local
  chown root:root /usr/local/bin/redis-local
}

redis_conf="/etc/redis/redis-local.conf"

log "Installing Redis packages"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  redis-server \
  redis-tools

mkdir -p /var/log/redis /var/lib/redis
chown redis:redis /var/log/redis /var/lib/redis

cat > "$redis_conf" <<'EOF_CONF'
bind 127.0.0.1 -::1
protected-mode yes
port 6379
tcp-backlog 511
timeout 0
tcp-keepalive 300
daemonize yes
supervised no
pidfile /var/run/redis-local.pid
loglevel notice
logfile /var/log/redis/redis-local.log
databases 16
dir /var/lib/redis
save 900 1
save 300 10
save 60 10000
appendonly no
EOF_CONF

log "Starting Redis"
if ! redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1; then
  redis-server "$redis_conf"
fi

install_helper
log "Done. Use 'redis-local status' or 'redis-local shell'."
