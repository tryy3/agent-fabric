# Agent guidance

Hosted agent control plane (Go) + Flutter cockpits. Surfaces speak **ACP**; catalog HTTP (`/v1`) configures agents, providers, projects, and sandbox overlay. Inference, tools, memory, and sandboxes stay on the server — clients do not own the agent loop.

Stack: Go **1.26** (`controlplane/go.mod`), Flutter/Dart (`client/`, sdk `^3.13.0`), Postgres, Nix flake + direnv. Prefer `nix develop` / direnv so `go`, `flutter`, `postgresql`, `sqlc`, and `goose` are on PATH.

## Read first

- `docs/architecture.md` — layers, turn lifecycle, sandbox vs ACP
- `docs/decisions.md` — accepted product/protocol choices
- `DESIGN.md` — UI tokens, typography, and patterns (required before UI/theme/layout work)

## Commands

Order matters for a live stack: Postgres → control plane (cwd with `sandbox.json`) → client.

```bash
docker compose up -d
# DATABASE_URL overrides sandbox.json; otherwise engine.databaseUrl is used
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane   # fatal if cwd lacks sandbox.json; loads ./sandbox.json from process cwd
cd client && flutter run -d chrome          # default ACP ws://localhost:8080/acp, catalog http://localhost:8080
```

CLI smoke (agent must already exist in catalog):

```bash
go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -agent-id AGENT_ID -prompt "hello"
```

Tests:

```bash
# controlplane unit/integration-with-fakes — needs `postgresql` on PATH (nix develop)
nix develop -c bash -lc 'go -C controlplane test ./...'
# docker/podman sandbox integration only
go -C controlplane test -tags=integration ./internal/sandbox/docker/
cd client && flutter test
cd client && flutter analyze   # or dart analyze
```

Flags/env that change process behavior: `-addr` and `DATABASE_URL` override `sandbox.json` listen/DB when set. Default listen `:8080`.

## Constraints

- **Two APIs:** Catalog HTTP for definitions/settings; ACP WebSocket `/acp` for chat. Do not invent “create agent” over ACP.
- **Client boundary:** Do not put model choice, backend tools, MCP secrets, system prompt, or canonical transcript ownership in the Flutter app. Plane overrides client-supplied MCP/cwd on `session/new` except true client-origin tools.
- **UI / theme / layout:** Before any UI, theme, or layout change, read `DESIGN.md` and match its tokens, typography, and patterns.
- **Sandbox split:** `sandbox.json` is **host engine only** (`databaseUrl`, `listenAddr`, `dataDir`, docker `runtime` / `binPath` / `identityPrefix`). Image, kind, workspace root, idle TTL live in catalog settings (`GET/PATCH /v1/settings`) and apply on the next prompt. Missing/invalid `sandbox.json` is fatal. Deprecated overlay keys in an old file migrate once into `plane_settings`, then are ignored for OpenOptions.
- **Execution origins:** Docker/local FS is sandbox-origin, never ACP `fs/*`.
- **Generated SQL:** Do not hand-edit `controlplane/internal/db/*.sql.go`, `db.go`, or `models.go` (sqlc). Change `queries/` + `sqlc.yaml`, then regenerate.
- **Secrets:** Provider `apiKey` values belong in catalog/Postgres via Settings or `/v1/providers` — not in git, client source, or `OPENAI_*` env (catalog replaces that path).
- **Tests:** Prefer fakes/scripted providers; do not call a real LLM in unit tests.

Human setup and curl catalog examples: `README.md`.
