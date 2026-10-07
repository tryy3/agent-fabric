#!/usr/bin/env bash
# Create connection -> refresh models -> assistant on the isolated plane. Prints ASSISTANT_ID and saves it to $RUN/assistant.id.
set -euo pipefail
. "$(dirname "$0")/env.sh"
id(){ sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1; }
J=(-H 'content-type: application/json')
CONN=$(curl -fsS "$CP/v1/inference/connections" "${J[@]}" -d "{\"name\":\"fake\",\"type\":\"openai_compatible\",\"baseUrl\":\"http://127.0.0.1:$LLM_PORT/v1\",\"apiKey\":\"sk-fake\"}" | id)
curl -fsS -X POST "$CP/v1/inference/connections/$CONN/models/refresh" >/dev/null
AS=$(curl -fsS "$CP/v1/assistants" "${J[@]}" -d "{\"name\":\"Verify\",\"inferenceConnectionId\":\"$CONN\",\"defaultModel\":\"fake-model\"}" | id)
echo "$AS" > "$RUN/assistant.id"; echo "ASSISTANT_ID=$AS CONNECTION_ID=$CONN"
