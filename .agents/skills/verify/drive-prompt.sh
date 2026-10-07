#!/usr/bin/env bash
# Usage: drive-prompt.sh "text"  -> sends one ACP turn to the isolated plane via acp-cli; saves transcript to $OUT/acp-<ts>.txt
set -euo pipefail
. "$(dirname "$0")/env.sh"
AS=$(cat "$RUN/assistant.id")
go -C "$REPO/controlplane" run ./cmd/acp-cli -addr "localhost:$CP_PORT" -assistant-id "$AS" -prompt "$1" | tee "$OUT/acp-$(date +%s).txt"
