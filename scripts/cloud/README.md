# Cloud agent environment

Scripts that bring a fresh Anthropic cloud-agent container (Ubuntu, root) to a
working stack: Go, Flutter, Chromium, dependencies, Postgres, control plane and
the Flutter web client.

**Environment setup:** in the cloud environment settings, set *Setup script* to

```bash
bash scripts/cloud/setup.sh
```

| Script | Purpose |
| --- | --- |
| `setup.sh` | Idempotent install/verify of toolchains + deps, then runs `up.sh`. Never fails the boot; prints a summary. First run ≈ 6 min, re-run ≈ 6 s. |
| `up.sh` | Start Postgres (docker compose, native fallback), control plane (`:8080`), Flutter web (`:8090`). Safe to re-run; use it if services died between sessions. |
| `status.sh` | Versions and service health. |
| `down.sh` | Stop control plane and client. |

Versions: Go comes from `controlplane/go.mod`; Flutter is `FLUTTER_VERSION`
(keep in sync with `.github/workflows/ci.yml`).

## Environment variables (all optional)

| Variable | Default | Effect |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable` | Overrides `config.json`. Point at an external DB with `DB_MODE=external`. |
| `DB_MODE` | `auto` | `auto` (docker, then native Postgres), `docker`, `native`, `external` |
| `CP_ADDR` | `:8080` | Control plane listen address |
| `CLIENT_PORT` | `8090` | Static Flutter web server port |
| `START_CONTROLPLANE` / `START_CLIENT` | `1` | Set `0` to skip |
| `FLUTTER_VERSION`, `GO_VERSION` | see above | Override toolchain versions |
| `CHROME_EXECUTABLE` | auto-detected Playwright chromium | Browser for tooling |

Logs: `/var/tmp/agent-fabric/logs`. Provider API keys are configured through
the catalog (`/v1/inference/connections`), never via env.

## Notes

- Docker needs `dockerd` to be startable in the container; `up.sh` starts it
  and the Postgres image comes from Docker Hub. If that is blocked by the
  network policy, it falls back to the apt-installed native Postgres.
- The web client is a **release build** served statically at
  `http://localhost:8090` (rebuilt by `up.sh` when `client/lib` changed, after
  stopping the client with `down.sh`). Drive it with Playwright using
  `CHROME_EXECUTABLE` (launch with `--no-sandbox` as root).
- The Go toolchain is fetched from `proxy.golang.org` (go.dev/dl and github.com
  may be blocked by network policy); Flutter from `storage.googleapis.com`.
