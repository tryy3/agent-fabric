#!/usr/bin/env bash
# Start (or restart if dead) Postgres, the control plane, and the Flutter web
# client. Idempotent: safe to run at every session start.
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
load_env
cd "$REPO_ROOT" || exit 1

docker_ready() { docker info >/dev/null 2>&1; }

start_dockerd() {
  docker_ready && return 0
  have dockerd || return 1
  log "starting dockerd"
  $SUDO sh -c "nohup dockerd >'$LOG_DIR/dockerd.log' 2>&1 &"
  wait_for 30 docker_ready
}

pg_ready() { psql "$DATABASE_URL" -c 'select 1' >/dev/null 2>&1; }

start_db_docker() {
  start_dockerd || return 1
  docker compose up -d postgres >"$LOG_DIR/compose.log" 2>&1 || { tail -3 "$LOG_DIR/compose.log"; return 1; }
  wait_for 60 pg_ready
}

start_db_native() {
  local bin; bin="$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)"
  if [ -z "$bin" ] && have apt-get; then
    $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql >/dev/null 2>&1
    bin="$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)"
  fi
  [ -n "$bin" ] || return 1
  $SUDO id postgres >/dev/null 2>&1 || return 1
  if [ ! -f "$PG_NATIVE_DATA/PG_VERSION" ]; then
    $SUDO mkdir -p "$PG_NATIVE_DATA" && $SUDO chown postgres "$PG_NATIVE_DATA"
    $SUDO -u postgres "$bin/initdb" -D "$PG_NATIVE_DATA" -A trust >/dev/null || return 1
  fi
  $SUDO -u postgres "$bin/pg_ctl" -D "$PG_NATIVE_DATA" status >/dev/null 2>&1 \
    || $SUDO -u postgres "$bin/pg_ctl" -D "$PG_NATIVE_DATA" -l "$LOG_DIR/postgres.log" -o "-p 5432" -w start >/dev/null || return 1
  $SUDO -u postgres psql -qtc "select 1 from pg_roles where rolname='agent'" | grep -q 1 \
    || $SUDO -u postgres psql -qc "create role agent login superuser password 'agent'" >/dev/null
  $SUDO -u postgres psql -qtc "select 1 from pg_database where datname='agentfabric'" | grep -q 1 \
    || $SUDO -u postgres createdb -O agent agentfabric
  wait_for 20 pg_ready
}

start_db() {
  if pg_ready; then log "postgres already reachable"; return 0; fi
  case "$DB_MODE" in
    external) warn "DB_MODE=external but DATABASE_URL unreachable"; return 1 ;;
    docker)   start_db_docker ;;
    native)   start_db_native ;;
    *)        start_db_docker || { warn "docker postgres unavailable; falling back to native"; start_db_native; } ;;
  esac
}

start_bg() { # start_bg <name> <pidfile> <logfile> <cmd...>
  local name="$1" pidf="$2" logf="$3"; shift 3
  if [ -f "$pidf" ] && kill -0 "$(cat "$pidf")" 2>/dev/null; then log "$name already running (pid $(cat "$pidf"))"; return 0; fi
  nohup setsid "$@" >"$logf" 2>&1 < /dev/null &
  echo $! > "$pidf"
}

start_controlplane() {
  [ "$START_CONTROLPLANE" = "1" ] || return 0
  local bin="$STATE_DIR/controlplane"
  [ -x "$bin" ] || go -C controlplane build -o "$bin" ./cmd/controlplane || return 1
  # Must run from a directory containing config.json.
  (cd "$REPO_ROOT/controlplane" && start_bg controlplane "$STATE_DIR/cp.pid" "$LOG_DIR/controlplane.log" "$bin" -addr "$CP_ADDR")
  wait_for 40 curl -fs "http://localhost:$CP_PORT/v1/settings" || { tail -5 "$LOG_DIR/controlplane.log"; return 1; }
}

start_client() {
  [ "$START_CLIENT" = "1" ] || return 0
  local web="$STATE_DIR/web"
  if [ ! -f "$web/index.html" ] || [ -n "$(find client/lib client/pubspec.yaml -newer "$web/index.html" -print -quit 2>/dev/null)" ]; then
    log "building flutter web (release)"
    (cd client && flutter build web --release --no-pub --output "$web" >"$LOG_DIR/flutter-build.log" 2>&1) \
      || { tail -10 "$LOG_DIR/flutter-build.log"; return 1; }
  fi
  start_bg client "$STATE_DIR/web.pid" "$LOG_DIR/client.log" \
    python3 -m http.server "$CLIENT_PORT" --bind 0.0.0.0 --directory "$web"
  wait_for 10 curl -fs "http://localhost:$CLIENT_PORT/"
}

rc=0
log "postgres";      start_db           || { warn "postgres failed"; rc=1; }
log "control plane"; start_controlplane || { warn "control plane failed (see $LOG_DIR/controlplane.log)"; rc=1; }
log "flutter web";   start_client       || { warn "flutter web failed (see $LOG_DIR/flutter-build.log)"; rc=1; }
exit $rc
