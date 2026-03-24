#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-postgres] must run as root" >&2
  exit 1
fi

log() {
  echo "[add-postgres] $*"
}

install_helper() {
  cat > /usr/local/bin/pg-local <<'EOF_HELPER'
#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -eq 0 ]; then
  exec /usr/local/bin/sand-privileged pg-local "$@"
fi

exec sudo /usr/local/bin/sand-privileged pg-local "$@"
EOF_HELPER
  chmod 0755 /usr/local/bin/pg-local
  chown root:root /usr/local/bin/pg-local
}

cluster_dir="$(find /etc/postgresql -mindepth 2 -maxdepth 2 -type d -name main 2>/dev/null | sort -V | tail -n1 || true)"

log "Installing PostgreSQL packages"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  postgresql \
  postgresql-client

if [ -z "$cluster_dir" ]; then
  cluster_dir="$(find /etc/postgresql -mindepth 2 -maxdepth 2 -type d -name main | sort -V | tail -n1)"
fi

if [ -z "$cluster_dir" ]; then
  echo "[add-postgres] Unable to determine PostgreSQL cluster path" >&2
  exit 1
fi

pg_version="$(basename "$(dirname "$cluster_dir")")"
pg_conf="/etc/postgresql/${pg_version}/main/postgresql.conf"
pg_hba="/etc/postgresql/${pg_version}/main/pg_hba.conf"

log "Configuring PostgreSQL cluster ${pg_version}/main for local-only access"
sed -ri "s/^#?listen_addresses\s*=.*/listen_addresses = '127.0.0.1'/" "$pg_conf"

if ! grep -q "# sand-local" "$pg_hba"; then
  tmp_file="$(mktemp)"
  {
    echo "host all all 127.0.0.1/32 trust # sand-local"
    echo "host all all ::1/128 trust # sand-local"
    cat "$pg_hba"
  } > "$tmp_file"
  install -m 0640 -o postgres -g postgres "$tmp_file" "$pg_hba"
  rm -f "$tmp_file"
fi

log "Starting PostgreSQL cluster"
if ! /usr/local/bin/sand-privileged pg-local start >/dev/null; then
  echo "[add-postgres] Failed to start PostgreSQL cluster ${pg_version}/main" >&2
  exit 1
fi
pg_ctlcluster "$pg_version" main reload >/dev/null 2>&1 || true

log "Ensuring role/database defaults exist"
if ! runuser -u postgres -- psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='node'" | grep -q 1; then
  runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c "CREATE ROLE node WITH LOGIN SUPERUSER CREATEDB"
fi

if ! runuser -u postgres -- psql -tAc "SELECT 1 FROM pg_database WHERE datname='app'" | grep -q 1; then
  runuser -u postgres -- createdb -O node app
fi

install_helper
log "Done. Use 'pg-local status' or 'pg-local shell'."
