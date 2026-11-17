#!/usr/bin/env bash
# Per-user PostgreSQL cluster for MORN1NGST4R (D-67, milestone M3).
#
#   pg-cluster.sh init        create the cluster (refuses if it exists)
#   pg-cluster.sh start       start it with pg_ctl and wait until it accepts connections
#   pg-cluster.sh bootstrap   create the four login roles and the database (refuses if they exist)
#   pg-cluster.sh stop        stop it (fast shutdown)
#   pg-cluster.sh status      print whether it runs
#   pg-cluster.sh paths       print the data and socket directories
#
# Rules from D-67 that this script keeps:
#   - PostgreSQL 18 from the user's own installation. Nothing is downloaded or bundled.
#   - The cluster lives under $XDG_DATA_HOME/morningstar/pg with owner-only permissions.
#   - It listens on a unix socket only (listen_addresses = '') in a directory with
#     mode 0700 and unix_socket_permissions 0700.
#   - Peer authentication only. pg_ident.conf maps the one OS user to four login
#     roles (ms_migrator, ms_api, ms_worker, ms_retention). No password exists, so
#     none can appear in an environment variable, an argument or a file.
#   - Every step fails closed. Nothing uses IF NOT EXISTS.
#
# MORNINGSTAR_PG_BASE moves the whole tree (the integration tests use it). It is a
# directory path and never a connection string.
set -euo pipefail
umask 077

base="${MORNINGSTAR_PG_BASE:-${XDG_DATA_HOME:-$HOME/.local/share}/morningstar}"
data="$base/pg"
sock="$base/pg-run"
db=morningstar
roles=(ms_migrator ms_api ms_worker ms_retention)

die() { echo "pg-cluster: $*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 not found. Install PostgreSQL 18 with the system package manager (D-67)"; }

check_version() {
  local v
  v="$(postgres --version | sed -E 's/^postgres \(PostgreSQL\) ([0-9]+).*/\1/')"
  [ "$v" = "18" ] || die "PostgreSQL 18 is required, found $v (D-67)"
}

check_paths() {
  [[ "$base" = /* ]] || die "the base directory must be absolute"
  # unix socket paths are limited to 107 bytes including the socket file name
  local longest="$sock/.s.PGSQL.5432"
  [ "${#longest}" -le 100 ] || die "the socket path is too long (${#longest} bytes). Set MORNINGSTAR_PG_BASE to a shorter directory"
}

pgctl() { pg_ctl -D "$data" "$@"; }

psql_admin() {
  # Peer authentication as the OS user, who owns the cluster (the bootstrap superuser).
  psql -X -q -v ON_ERROR_STOP=1 -h "$sock" -d postgres "$@"
}

cmd_init() {
  need initdb; need pg_ctl; need postgres; check_version; check_paths
  [ ! -e "$data" ] || die "the cluster $data exists already. init refuses to touch it"
  mkdir -p -m 0700 "$base" "$sock"
  chmod 0700 "$base" "$sock"
  initdb -D "$data" --auth-local=peer --auth-host=reject --encoding=UTF8 --no-locale >/dev/null
  chmod 0700 "$data"
  local os; os="$(id -un)"
  # Peer authentication through one map. The OS user may connect as itself (the
  # bootstrap superuser) and as each of the four roles.
  {
    echo "# MORN1NGST4R: peer authentication only (D-67)"
    echo "local   all   all   peer map=morningstar"
  } >"$data/pg_hba.conf"
  {
    echo "# MAPNAME   SYSTEM-USERNAME   DATABASE-USERNAME"
    echo "morningstar   \"$os\"   \"$os\""
    for r in "${roles[@]}"; do echo "morningstar   \"$os\"   $r"; done
  } >"$data/pg_ident.conf"
  cat >>"$data/postgresql.conf" <<EOF

# MORN1NGST4R (D-67). Memory measured in milestone M3 on the 7.7 GB reference
# laptop with these values (PostgreSQL 18.6, proportional set size, which counts
# shared memory once): idle cluster 7 MB, 10 MB more with the server connected
# (one connection), 59 MB with 27 connections open. The summed resident size is
# larger (24 MB idle, 438 MB with 27 connections) because every process counts the
# shared buffers it touched. Nothing here was tuned further.
listen_addresses = ''
unix_socket_directories = '$sock'
unix_socket_permissions = 0700
max_connections = 30
shared_buffers = 64MB
work_mem = 4MB
maintenance_work_mem = 32MB
wal_buffers = 4MB
max_wal_size = 256MB
autovacuum_max_workers = 2
ssl = off
password_encryption = scram-sha-256
log_destination = 'stderr'
logging_collector = off
log_connections = off
log_line_prefix = '%m [%p] '
EOF
  chmod 0600 "$data/pg_hba.conf" "$data/pg_ident.conf" "$data/postgresql.conf"
  echo "pg-cluster: created $data (socket directory $sock)"
}

cmd_start() {
  need pg_ctl
  [ -d "$data" ] || die "no cluster at $data. Run init first"
  pgctl -w -t 30 -l "$base/pg.log" start >/dev/null
  echo "pg-cluster: started"
}

cmd_bootstrap() {
  need psql
  [ -S "$sock/.s.PGSQL.5432" ] || die "the cluster is not running. Run start first"
  # The roles have no password and no extra privilege. They can log in only
  # through the peer map. ms_migrator owns the database and, through the
  # migrations, every object in it.
  psql_admin <<EOF
CREATE ROLE ms_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE ms_api       LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE ms_worker    LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE ms_retention LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE DATABASE $db OWNER ms_migrator ENCODING 'UTF8' TEMPLATE template0;
REVOKE ALL ON DATABASE $db FROM PUBLIC;
GRANT CONNECT ON DATABASE $db TO ms_api, ms_worker, ms_retention;
EOF
  echo "pg-cluster: roles and database $db created"
}

cmd_stop() {
  need pg_ctl
  [ -d "$data" ] || die "no cluster at $data"
  pgctl -m fast -w stop >/dev/null
  echo "pg-cluster: stopped"
}

cmd_status() {
  need pg_ctl
  [ -d "$data" ] || { echo "pg-cluster: no cluster at $data"; exit 3; }
  pgctl status >/dev/null && echo "pg-cluster: running" || { echo "pg-cluster: stopped"; exit 3; }
}

case "${1:-}" in
  init) cmd_init ;;
  start) cmd_start ;;
  bootstrap) cmd_bootstrap ;;
  stop) cmd_stop ;;
  status) cmd_status ;;
  paths) echo "data=$data"; echo "socket=$sock" ;;
  *) die "usage: pg-cluster.sh init|start|bootstrap|stop|status|paths" ;;
esac
