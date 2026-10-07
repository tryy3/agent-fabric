#!/usr/bin/env bash
# Stop only what up.sh started (pidfile, then the owner of our verify port), drop the verify DB and scratch dir. Evidence in $OUT is kept.
. "$(dirname "$0")/env.sh"
owner(){ ss -ltnp "sport = :$1" 2>/dev/null | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1; }
stop(){ # name port
  local p; p=$(cat "$RUN/$1.pid" 2>/dev/null); [ -z "$p" ] && p=$(owner "$2")
  [ -n "$p" ] && kill "$p" 2>/dev/null
  for _ in $(seq 20); do [ -z "$(owner "$2")" ] && return 0; sleep 0.25; done
  p=$(owner "$2"); [ -n "$p" ] && kill "$p" 2>/dev/null
}
stop controlplane "$CP_PORT"; stop fake-llm "$LLM_PORT"
psql "$PG_ADMIN" -qc "drop database if exists $DB_NAME with (force)" 2>/dev/null
rm -rf "$RUN"; echo "down; evidence kept in $OUT"
