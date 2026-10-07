#!/usr/bin/env bash
# Read-only: is the isolated verification instance worth driving?
. "$(dirname "$0")/env.sh"
rc=0
chk(){ if eval "$2" >/dev/null 2>&1; then echo "ok   $1"; else echo "FAIL $1"; rc=1; fi; }
chk "postgres :5432"            "psql '$PG_ADMIN' -Atc 'select 1'"
chk "db $DB_NAME exists"        "psql '$PG_ADMIN' -Atc \"select 1 from pg_database where datname='$DB_NAME'\" | grep -q 1"
chk "plane pid is ours + alive" "kill -0 \$(cat '$RUN/controlplane.pid')"
chk "plane answers $CP"         "curl -fsS $CP/v1/settings"
chk "fake-llm answers :$LLM_PORT" "curl -fsS http://127.0.0.1:$LLM_PORT/v1/models"
chk "not the dev plane (:8080 untouched)" "[ '$CP_PORT' != 8080 ]"
exit $rc
