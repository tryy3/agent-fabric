#!/usr/bin/env bash
# Run controlplane Go tests. The dbtest helper starts a throwaway Postgres via
# initdb/postgres, which refuse to run as root, so as root this script runs the
# tests as an unprivileged user (created by setup.sh). Usage: test-go.sh [go test args]
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
load_env
pgbin="$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)"
[ -n "$pgbin" ] && export PATH="$pgbin:$PATH"
args=("$@"); [ ${#args[@]} -eq 0 ] && args=(./...)
if [ "$(id -u)" -ne 0 ]; then
  exec go -C "$REPO_ROOT/controlplane" test "${args[@]}"
fi
id "$TEST_USER" >/dev/null 2>&1 || useradd -m -s /bin/bash "$TEST_USER"
chmod o+rx "$(dirname "$GO_HOME")" 2>/dev/null
# -p keeps proxy/CA env vars; HOME/cache dirs are the test user's own.
exec runuser -u "$TEST_USER" -p -- env HOME="/home/$TEST_USER" \
  GOCACHE="/home/$TEST_USER/.cache/go-build" GOMODCACHE="/home/$TEST_USER/go/pkg/mod" \
  GOFLAGS=-buildvcs=false \
  go -C "$REPO_ROOT/controlplane" test "${args[@]}"
