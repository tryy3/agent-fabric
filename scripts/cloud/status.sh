#!/usr/bin/env bash
# Print versions and service health.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
load_env
ok() { printf '  %-14s %s\n' "$1" "$2"; }
ok go "$(go env GOVERSION 2>/dev/null || echo MISSING) (need >= $GO_VERSION)"
ok flutter "$(flutter_version | grep . || echo MISSING) (need $FLUTTER_VERSION)"
ok chromium "$(find_chromium || echo MISSING)"
ok docker "$(docker info >/dev/null 2>&1 && echo up || echo 'daemon down')"
ok postgres "$(psql "$DATABASE_URL" -tc 'select 1' >/dev/null 2>&1 && echo reachable || echo DOWN)"
ok controlplane "$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$CP_PORT/v1/settings" 2>/dev/null) http://localhost:$CP_PORT  (ACP ws://localhost:$CP_PORT/acp)"
ok client "$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$CLIENT_PORT/" 2>/dev/null) http://localhost:$CLIENT_PORT"
ok logs "$LOG_DIR"
