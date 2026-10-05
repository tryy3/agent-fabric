#!/usr/bin/env bash
# Stop control plane + client (Postgres is left running; `docker compose down` to remove).
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
for n in cp web; do
  if [ -f "${STATE_DIR:?}/$n.pid" ]; then kill "$(cat "$STATE_DIR/$n.pid")" 2>/dev/null; rm -f "${STATE_DIR:?}/${n:?}.pid"; fi
done
