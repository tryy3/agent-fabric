#!/usr/bin/env bash
# Shared settings for the cloud-agent scripts. Every value can be overridden by
# an environment variable set in the cloud environment configuration.
# shellcheck disable=SC2034

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Toolchain versions (keep FLUTTER_VERSION in sync with .github/workflows/ci.yml;
# the Go version comes from controlplane/go.mod).
FLUTTER_VERSION="${FLUTTER_VERSION:-3.47.0}"
FLUTTER_HOME="${FLUTTER_HOME:-/opt/flutter}"
GO_MOD_VERSION="$(awk '/^go /{print $2}' "$REPO_ROOT/controlplane/go.mod" 2>/dev/null)"
GO_VERSION="${GO_VERSION:-${GO_MOD_VERSION:-1.26.0}}"
GO_HOME="${GO_HOME:-/opt/go-toolchain}"

# Local database (docker compose service `postgres`; native Postgres fallback).
export DATABASE_URL="${DATABASE_URL:-postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable}"
DB_MODE="${DB_MODE:-auto}"            # auto | docker | native | external
PG_NATIVE_DATA="${PG_NATIVE_DATA:-/var/lib/agentfabric-pg}"

# Control plane + client.
CP_ADDR="${CP_ADDR:-:8080}"
CP_PORT="${CP_ADDR##*:}"
CLIENT_PORT="${CLIENT_PORT:-8090}"
START_CONTROLPLANE="${START_CONTROLPLANE:-1}"
START_CLIENT="${START_CLIENT:-1}"

# Chromium (Playwright's preinstalled build is used when present).
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-/opt/pw-browsers}"

STATE_DIR="${STATE_DIR:-/var/tmp/agent-fabric}"
LOG_DIR="$STATE_DIR/logs"
mkdir -p "$LOG_DIR"

log()  { printf '\033[1;34m[cloud-setup]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[cloud-setup] WARN:\033[0m %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }
SUDO=""; if [ "$(id -u)" -ne 0 ] && have sudo; then SUDO="sudo"; fi

# Succeeds when installed version ($1) >= wanted ($2).
version_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }

find_chromium() {
  local c
  for c in "${CHROME_EXECUTABLE:-}" \
    "$PLAYWRIGHT_BROWSERS_PATH"/chromium-*/chrome-linux*/chrome \
    "$(command -v chromium 2>/dev/null)" "$(command -v chromium-browser 2>/dev/null)" \
    "$(command -v google-chrome 2>/dev/null)"; do
    if [ -n "$c" ] && [ -x "$c" ]; then echo "$c"; return 0; fi
  done
  return 1
}

wait_for() { # wait_for <seconds> <cmd...>
  local n="$1"; shift
  for _ in $(seq "$n"); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done
  return 1
}

flutter_version() { flutter --version --machine 2>/dev/null | sed -n 's/.*"frameworkVersion": *"\([^"]*\)".*/\1/p'; }

load_env() {
  [ -d "$GO_HOME/bin" ] && export PATH="$GO_HOME/bin:$PATH"
  [ -d "$FLUTTER_HOME/bin" ] && export PATH="$FLUTTER_HOME/bin:$PATH"
  export PATH="$HOME/go/bin:$HOME/.pub-cache/bin:$PATH"
  export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
  export FLUTTER_SUPPRESS_ANALYTICS=true DART_SUPPRESS_ANALYTICS=true
  local c
  if c="$(find_chromium)"; then export CHROME_EXECUTABLE="$c"; fi
  return 0
}
