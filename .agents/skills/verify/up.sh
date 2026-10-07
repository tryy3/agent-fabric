#!/usr/bin/env bash
# Start an isolated plane (+ fake LLM) for verification. Idempotent. Needs Postgres on :5432 (docker compose up -d).
set -euo pipefail
. "$(dirname "$0")/env.sh"
psql "$PG_ADMIN" -Atc "select 1" >/dev/null || { echo "Postgres not reachable on :5432 (run: docker compose up -d)"; exit 1; }
psql "$PG_ADMIN" -Atc "select 1 from pg_database where datname='$DB_NAME'" | grep -q 1 \
  || psql "$PG_ADMIN" -qc "create database $DB_NAME"
cat > "$RUN/config.json" <<JSON
{"databaseUrl":"","listenAddr":":$CP_PORT","dataDir":"$RUN/data","docker":{"runtime":"auto","binPath":"","identityPrefix":"afverify-"}}
JSON
if ! curl -fsS "http://127.0.0.1:$LLM_PORT/v1/models" >/dev/null 2>&1; then
  (cd "$SKILL/fake-llm" && go build -o "$RUN/fake-llm" main.go)
  nohup "$RUN/fake-llm" -addr "127.0.0.1:$LLM_PORT" >"$RUN/fake-llm.log" 2>&1 &
  echo $! > "$RUN/fake-llm.pid"
fi
if ! curl -fsS "$CP/v1/settings" >/dev/null 2>&1; then
  go -C "$REPO/controlplane" build -o "$RUN/controlplane" ./cmd/controlplane
  (cd "$RUN" && DATABASE_URL="$DATABASE_URL" nohup ./controlplane -addr ":$CP_PORT" >"$RUN/controlplane.log" 2>&1 & echo $! > "$RUN/controlplane.pid")
fi
for _ in $(seq 40); do curl -fsS "$CP/v1/settings" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "$CP/v1/settings" >/dev/null || { echo "plane not ready; see $RUN/controlplane.log"; tail -20 "$RUN/controlplane.log"; exit 1; }
echo "ready: plane $CP  fake-llm :$LLM_PORT  db $DB_NAME  logs $RUN"
