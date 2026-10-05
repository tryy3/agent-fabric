#!/usr/bin/env bash
# Idempotent environment bootstrap for Anthropic cloud agents.
# Paste into the environment's "Setup script" field as:
#     bash scripts/cloud/setup.sh
# Installs/validates Go, Flutter, Chromium, apt libs, Go modules and pub
# packages. It does NOT start any services: run up.sh when you need Postgres,
# the control plane and the web client, and down.sh when done.
# Failures in optional steps warn instead of aborting.
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
cd "$REPO_ROOT" || exit 0
FAILED=()
step() { local name="$1"; shift; log "== $name"; "$@" || { warn "$name failed"; FAILED+=("$name"); }; }

install_apt() {
  local pkgs=(curl ca-certificates git unzip xz-utils zip libglu1-mesa
    clang cmake ninja-build pkg-config libgtk-3-dev  # flutter linux desktop / cgo
    gcc libc6-dev postgresql-client python3
    fonts-liberation libnss3 libatk-bridge2.0-0 libgbm1 libasound2t64 libxkbcommon0)
  local missing=() p
  for p in "${pkgs[@]}"; do dpkg -s "$p" >/dev/null 2>&1 || missing+=("$p"); done
  if [ ${#missing[@]} -eq 0 ]; then log "apt packages present"; return 0; fi
  have apt-get || { warn "no apt-get; skipping ${missing[*]}"; return 0; }
  $SUDO apt-get update -qq || warn "apt-get update failed"
  # Fall back to one-by-one so a single unavailable package doesn't block the rest.
  $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends "${missing[@]}" >/dev/null 2>&1 \
    || for p in "${missing[@]}"; do
         $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends "$p" >/dev/null 2>&1 || warn "apt: $p unavailable"
       done
  return 0
}

install_go() {
  load_env
  local cur=""; have go && cur="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  if [ -n "$cur" ] && version_ge "$cur" "$GO_VERSION"; then log "go $cur ok (need >= $GO_VERSION)"; return 0; fi
  log "go ${cur:-missing}; need $GO_VERSION"
  # Prefer the module proxy (works behind restrictive egress policies); fall back to go.dev.
  local arch tmp; arch="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
  tmp="$(mktemp -d)"
  if curl -fsSL "https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go${GO_VERSION}.linux-${arch}.zip" -o "$tmp/go.zip" \
     && unzip -q "$tmp/go.zip" -d "$tmp"; then
    rm -rf "${GO_HOME:?}"; mkdir -p "$GO_HOME"
    cp -a "$tmp"/golang.org/toolchain@*/. "$GO_HOME/"
  elif curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" | tar -xz -C "$tmp"; then
    rm -rf "${GO_HOME:?}"; mv "$tmp/go" "$GO_HOME"
  else
    warn "direct Go download failed; relying on GOTOOLCHAIN=auto from go.mod"
  fi
  rm -rf "${tmp:?}"; load_env
  return 0
}

install_flutter() {
  load_env
  local cur=""; have flutter && cur="$(flutter_version)"
  if [ "$cur" = "$FLUTTER_VERSION" ]; then log "flutter $cur ok"; else
    log "flutter ${cur:-missing}; need $FLUTTER_VERSION"
    local url="https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz"
    local tmp; tmp="$(mktemp -d)"
    curl -fsSL "$url" -o "$tmp/flutter.tar.xz" || { rm -rf "${tmp:?}"; return 1; }
    rm -rf "${FLUTTER_HOME:?}"; mkdir -p "$(dirname "$FLUTTER_HOME")"
    tar -xJf "$tmp/flutter.tar.xz" -C "$tmp" && mv "$tmp/flutter" "$FLUTTER_HOME"
    rm -rf "${tmp:?}"; load_env
  fi
  git config --global --add safe.directory "$FLUTTER_HOME" 2>/dev/null || true
  flutter config --no-analytics --enable-web >/dev/null 2>&1 || true
  flutter precache --web >/dev/null 2>&1 || warn "flutter precache --web failed"
  return 0
}

check_chromium() {
  load_env
  if c="$(find_chromium)"; then log "chromium: $c"; return 0; fi
  log "chromium missing; trying playwright / apt"
  if have npx && npx -y playwright install --with-deps chromium >/dev/null 2>&1; then load_env; find_chromium >/dev/null && return 0; fi
  if have apt-get; then
    $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq chromium >/dev/null 2>&1 \
      || $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq chromium-browser >/dev/null 2>&1
  fi
  load_env; find_chromium >/dev/null
}

deps_go() {
  load_env
  go -C controlplane mod download || return 1
  go -C controlplane build -o "$STATE_DIR/controlplane" ./cmd/controlplane || return 1
  # Optional codegen/migration CLIs from the flake; not needed to run or test.
  have sqlc  || go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest >/dev/null 2>&1 || warn "sqlc not installed (optional)"
  have goose || go install github.com/pressly/goose/v3/cmd/goose@latest >/dev/null 2>&1 || warn "goose not installed (optional)"
  return 0
}

deps_flutter() { load_env; (cd client && flutter pub get); }

deps_node() {
  # prompt-scrub (capture scrubbing, see CLAUDE.md); mirrors flake.nix shellHook.
  have npm || return 0
  have prompt-scrub && return 0
  npm_config_prefix="$REPO_ROOT/.npm-global" npm install -g @nanocollective/prompt-scrub >/dev/null 2>&1 \
    || warn "prompt-scrub install failed (optional)"
  return 0
}

persist_env() {
  local f=/etc/profile.d/agent-fabric.sh c
  [ -w /etc/profile.d ] || f="$HOME/.agent-fabric-env.sh"
  {
    echo "# generated by scripts/cloud/setup.sh"
    echo "export PATH=\"$GO_HOME/bin:$FLUTTER_HOME/bin:\$HOME/go/bin:\$HOME/.pub-cache/bin:\$PATH\""
    echo "export GOTOOLCHAIN=auto FLUTTER_SUPPRESS_ANALYTICS=true DART_SUPPRESS_ANALYTICS=true"
    echo "export DATABASE_URL='$DATABASE_URL'"
    echo "export PLAYWRIGHT_BROWSERS_PATH='$PLAYWRIGHT_BROWSERS_PATH'"
    if c="$(find_chromium)"; then echo "export CHROME_EXECUTABLE='$c'"; fi
    if [ -x "$REPO_ROOT/.npm-global/bin/prompt-scrub" ]; then
      echo "export PATH=\"$REPO_ROOT/.npm-global/bin:\$PATH\""
      echo "export PROMPT_SCRUB_BIN=\"$(readlink -f "$REPO_ROOT/.npm-global/bin/prompt-scrub")\""
    fi
  } > "$f"
  if [ "$f" = "$HOME/.agent-fabric-env.sh" ] && ! grep -qs agent-fabric-env "$HOME/.bashrc"; then
    echo "source $f" >> "$HOME/.bashrc"
  fi
  return 0
}

step "apt packages"   install_apt
step "go"             install_go
step "flutter"        install_flutter
step "chromium"       check_chromium
step "go modules"     deps_go
step "flutter pub"    deps_flutter
step "node tools"     deps_node
ensure_test_user() {
  [ "$(id -u)" -eq 0 ] || return 0
  id "$TEST_USER" >/dev/null 2>&1 || useradd -m -s /bin/bash "$TEST_USER"
}

step "test user"      ensure_test_user
step "persist env"    persist_env

log "summary"
bash "$REPO_ROOT/scripts/cloud/status.sh" || true
log "services are NOT started; run: bash scripts/cloud/up.sh (and down.sh when done)"
if [ ${#FAILED[@]} -gt 0 ]; then warn "failed steps: ${FAILED[*]}"; fi
# Never fail the environment boot because an optional step failed.
exit 0
