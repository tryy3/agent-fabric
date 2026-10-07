# Sourced by the verify scripts. Isolated instance: own DB, ports, workdir; never touches the dev DB or :8080.
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SKILL="$REPO/.claude/skills/verify"
RUN="${VERIFY_RUN:-/tmp/af-verify/run}"          # scratch: pids, logs, config, data (removed by down.sh)
OUT="${VERIFY_OUT:-/tmp/af-verify/evidence}"      # proof artifacts (survive down.sh)
CP_PORT="${VERIFY_CP_PORT:-8099}"
LLM_PORT="${VERIFY_LLM_PORT:-8098}"
PG_ADMIN="postgres://agent:agent@localhost:5432/postgres?sslmode=disable"
DB_NAME="agentfabric_verify"
DATABASE_URL="postgres://agent:agent@localhost:5432/$DB_NAME?sslmode=disable"
CP="http://localhost:$CP_PORT"
mkdir -p "$RUN" "$OUT"
