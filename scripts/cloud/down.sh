#!/usr/bin/env bash
# Stop the control plane and the Flutter web client. Pass --db to also stop
# Postgres (docker compose stop; data is kept).
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
for n in cp web; do
  if [ -f "${STATE_DIR:?}/$n.pid" ]; then
    kill -- "-$(cat "$STATE_DIR/$n.pid")" 2>/dev/null || kill "$(cat "$STATE_DIR/$n.pid")" 2>/dev/null
    rm -f "${STATE_DIR:?}/${n:?}.pid"
  fi
done
if [ "${1:-}" = "--db" ]; then
  (cd "$REPO_ROOT" && docker compose stop postgres 2>/dev/null)
  for bin in /usr/lib/postgresql/*/bin; do
    [ -d "$PG_NATIVE_DATA" ] && $SUDO -u postgres "$bin/pg_ctl" -D "$PG_NATIVE_DATA" stop 2>/dev/null
  done
fi
log "stopped"
