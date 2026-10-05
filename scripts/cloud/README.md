# Cloud agent environment

Scripts for Anthropic cloud-agent containers (Ubuntu, root). **Read this
file first when running in a cloud environment.**

## Workflow

1. **Setup (automatic).** The environment's *Setup script* field takes the
   script **body** (inline Bash, not a file path), run as root before Claude
   starts. Use:

   ```bash
   #!/bin/bash
   # Docs don't specify the working directory; look in cwd, then the usual clone path.
   for d in "$PWD" /home/user/agent-fabric; do
     if [ -f "$d/scripts/cloud/setup.sh" ]; then bash "$d/scripts/cloud/setup.sh"; exit 0; fi
   done
   echo "scripts/cloud/setup.sh not found (repo not cloned yet?)" >&2
   exit 0
   ```

   `setup.sh` only installs/verifies tools — Go (version from
   `controlplane/go.mod`), Flutter (`FLUTTER_VERSION`, same as
   `.github/workflows/ci.yml`), Chromium, apt libraries, Go modules, pub
   packages — is idempotent, always exits 0 and **starts no services**.
   Anthropic snapshots the filesystem if setup finishes in ≈ 5 minutes and
   reuses it (setup is skipped for later sessions until the script, allowed
   hosts or the ≈ 7-day expiry change); background processes are not part of
   the snapshot, which is why services are started on demand with `up.sh`.
   **PATH:** the image's `/usr/local/go` (1.24) precedes the toolchains
   installed to `/opt`, and `/etc/profile.d` only reaches login shells. The
   `SessionStart` hook in `.claude/settings.json` runs `session-env.sh`, which
   exports the right `PATH`, `DATABASE_URL`, `CHROME_EXECUTABLE` etc. into the
   agent's shell (cloud only). Without it, scripts still work (they set their
   own PATH) but a bare `go`/`flutter` would not.
2. **Develop and run tests without services.** Go/Flutter tests do not need the
   stack up:

   ```bash
   bash scripts/cloud/test-go.sh                 # all; or: test-go.sh ./internal/db/...
   # (plain `go test` fails as root: the dbtest helper runs initdb, which refuses root.
   #  test-go.sh runs it as the unprivileged user `afdev` with Postgres binaries on PATH)
   cd client && dart format --output=none --set-exit-if-changed . \
     && flutter analyze --fatal-infos \
     && flutter test --test-randomize-ordering-seed random
   ```
3. **Start the stack only when you must exercise the running harness**
   (ACP/catalog smoke, browser-driving the client):

   ```bash
   bash scripts/cloud/up.sh      # Postgres :5432, control plane :8080, Flutter web :8090
   bash scripts/cloud/status.sh  # versions + health
   ```
   Then e.g. `go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -assistant-id ID -prompt hi`,
   or drive `http://localhost:8090` with Playwright (`CHROME_EXECUTABLE`, launch with `--no-sandbox`).
4. **When finished, shut down:** `bash scripts/cloud/down.sh` (control plane +
   client; add `--db` to also stop Postgres). Don't leave services running while
   you are still just editing code.

`up.sh` starts Postgres via `docker compose` (starting `dockerd` if needed) and
falls back to the apt-installed native Postgres. The web client is a release
build served statically; `up.sh` rebuilds it when `client/lib` changed (restart
with `down.sh` then `up.sh`). Logs: `/var/tmp/agent-fabric/logs`.

## Environment variables (all optional)

Set these in the cloud environment configuration.

| Variable | Default | Effect |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable` | Overrides `config.json`. Use with `DB_MODE=external` for a remote DB. |
| `DB_MODE` | `auto` | `auto` (docker → native), `docker`, `native`, `external` |
| `CP_ADDR` | `:8080` | Control plane listen address |
| `CLIENT_PORT` | `8090` | Static Flutter web port |
| `START_CONTROLPLANE` / `START_CLIENT` | `1` | `0` makes `up.sh` skip that service |
| `FLUTTER_VERSION` / `GO_VERSION` | CI pin / `go.mod` | Toolchain overrides |
| `FLUTTER_HOME` / `GO_HOME` | `/opt/flutter` / `/opt/go-toolchain` | Install locations |
| `CHROME_EXECUTABLE` | auto (Playwright chromium) | Browser for tooling |
| `PLAYWRIGHT_BROWSERS_PATH` | `/opt/pw-browsers` | Where Chromium is looked up |
| `PG_NATIVE_DATA` | `/var/lib/agentfabric-pg` | Data dir for the native Postgres fallback |
| `TEST_USER` | `afdev` | Unprivileged user `test-go.sh` runs Go tests as |
| `STATE_DIR` | `/var/tmp/agent-fabric` | pids, logs, built web bundle |

Provider API keys go through the catalog (`/v1/inference/connections`), never env.

## Network allowlist

Add to the environment's trusted domains so nothing needs debugging later:

| Domain | Used for |
| --- | --- |
| `proxy.golang.org`, `sum.golang.org` | Go modules, Go toolchain download, checksum DB |
| `storage.googleapis.com` | Flutter SDK, Dart SDK, web engine artifacts |
| `pub.dev`, `*.pub.dev` | Dart/Flutter packages |
| `archive.ubuntu.com`, `security.ubuntu.com` | apt packages |
| `registry-1.docker.io`, `auth.docker.io`, `production.cloudflare.docker.com` | Docker Hub pulls (`postgres:18-alpine`, sandbox images) |
| `registry.npmjs.org` | `prompt-scrub`, Playwright |
| `github.com`, `objects.githubusercontent.com`, `raw.githubusercontent.com` | git, Go `direct` fetches, `go install` fallbacks |
| `ghcr.io` | Only if pulling project images (`ghcr.io/tryy3/agent-fabric/*`) |
| `go.dev`, `dl.google.com` | Only the fallback Go download |
| `cdn.playwright.dev`, `playwright.azureedge.net` | Only if Chromium must be reinstalled |
| `fonts.gstatic.com`, `fonts.googleapis.com` | Flutter web fonts when driving the client in a browser |
| Inference provider hosts (e.g. `api.openai.com`, `api.anthropic.com`) | Only for live provider testing |

## Known benign noise

- Flutter warns about running as root.
- `dockerd` logs nftables / snapshotter "skip plugin" errors; Postgres is fine.
- `connections reload failed: CatalogException(400)` in `flutter test` output is an intentional test case.
