# Agent guidance

Hosted agent control plane (Go) + Flutter cockpits. Surfaces speak **ACP**; catalog HTTP (`/v1`) configures assistants, inference connections, projects, and execution settings. Inference, tools, memory, and sandboxes stay on the server — clients do not own the agent loop.

Stack: Go **1.26** (`controlplane/go.mod`), Flutter/Dart (`client/`, sdk `^3.13.0`), Postgres, Nix flake + direnv. Prefer `nix develop` / direnv so `go`, `flutter`, `postgresql`, `sqlc`, and `goose` are on PATH.

## Read first

- `docs/terminology.md` (sv: `docs/terminology.sv.md`) — canonical product vocabulary; keep code, APIs, UI, and docs aligned with it
- `docs/architecture.md` — layers, turn lifecycle, sandbox vs ACP
- `docs/decisions.md` — accepted product/protocol choices
- `DESIGN.md` — UI tokens, typography, and patterns (required before UI/theme/layout work)

## Commands

Order matters for a live stack: Postgres → control plane (cwd with `config.json`) → client.

```bash
docker compose up -d
# DATABASE_URL overrides config.json; otherwise engine.databaseUrl is used
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane   # fatal if cwd lacks config.json; loads ./config.json from process cwd
cd client && flutter run -d chrome          # default ACP ws://localhost:8080/acp, catalog http://localhost:8080
```

CLI smoke (agent must already exist in catalog):

```bash
go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -assistant-id ASSISTANT_ID -prompt "hello"
```

Tests:

```bash
# controlplane unit/integration-with-fakes — needs `postgresql` on PATH (nix develop)
nix develop -c bash -lc 'go -C controlplane test ./...'
# docker/podman sandbox integration only
go -C controlplane test -tags=integration ./internal/sandbox/docker/
cd client && dart format --output=none --set-exit-if-changed .
cd client && flutter analyze --fatal-infos
cd client && flutter test --test-randomize-ordering-seed random
```

Client CI mirrors those three gates in `.github/workflows/ci.yml` (Flutter 3.47.0).

Flags/env that change process behavior: `-addr` and `DATABASE_URL` override `config.json` listen/DB when set. Default listen `:8080`.

## Constraints

- **Terminology:** Before naming types, fields, routes, UI labels, or logs, check `docs/terminology.md` for an existing or near-match term and reuse it. When you introduce a new product concept (or change the meaning of an old one), update `docs/terminology.md` and `docs/terminology.sv.md` in the same change so the documents stay the source of truth. Do not leave the codebase and terminology out of sync.
- **Two APIs:** Catalog HTTP for definitions/settings; ACP WebSocket `/acp` for chat. Do not invent “create assistant” over ACP.
- **Client boundary:** Do not put model choice, backend tools, MCP secrets, system prompt, or canonical transcript ownership in the Flutter app. Plane overrides client-supplied MCP/cwd on `session/new` except true client-origin tools.
- **UI / theme / layout:** Before any UI, theme, or layout change, read `DESIGN.md` and match its tokens, typography, and patterns.
- **Sandbox split:** `config.json` is **host engine only** (`databaseUrl`, `listenAddr`, `dataDir`, docker `runtime` / `binPath` / `identityPrefix`). Image, kind, project root, idle TTL live in catalog settings (`GET/PATCH /v1/settings`) and apply on the next prompt. Missing/invalid `config.json` is fatal. Deprecated overlay keys in an old file migrate once into `plane_settings`, then are ignored for OpenOptions.
- **Execution origins:** Docker/local FS is environment-origin, never ACP `fs/*`.
- **Generated SQL:** Do not hand-edit `controlplane/internal/db/*.sql.go`, `db.go`, or `models.go` (sqlc). Change `queries/` + `sqlc.yaml`, then regenerate.
- **Secrets:** Provider `apiKey` values belong in catalog/Postgres via Settings or `/v1/inference/connections` — not in git, client source, or `OPENAI_*` env (catalog replaces that path).
- **Inference settings:** Generation knobs (`temperature`, max output, reasoning effort, provider-specific samplers) live on the **assistant** as `settings.inference`, merged via catalog PATCH and **pinned at `session/new`**. Live sessions keep the pin until a new session. Do not put sampling ownership in the Flutter agent loop or ACP `configOptions` unless explicitly designed. **When adding or changing a provider type:** investigate that provider’s current API and model matrix; expose the recommended general knobs plus any high-value provider-specific fields; extend decode/validation, adapter request mapping, and the Assistants UI together (omitempty — omit unset fields so upstream defaults apply). Some cloud models reject non-default sampling; leave fields unset rather than inventing per-model strip lists unless documented for that adapter.
- **Inter-service capture:** Persist traffic between plane-owned hops as first-class data (logging + chat inspector), not only ephemeral process logs. Prefer writing a capture when the hop completes (or once per tool-loop round). **HTTP** (provider inference, outbound catalog/MCP calls, etc.): store method, URL, request/response headers, and bodies. **WebSocket / ACP / other streaming transports:** persist message payloads across rounds; full frames when practical. **Scrub before persist:** never write raw secrets/PII into the capture store. Prefer [Nano Collective `prompt-scrub`](https://github.com/Nano-Collective/prompt-scrubber) for body/content scrubbing (stable placeholders); always apply plane-side header/cookie redaction too (`Authorization`, `apiKey`, bearer tokens, `Set-Cookie` / `Cookie`) — prompt-scrub is content-layer only and does not cover the network/header layer. Capture scrubbing is one-way at rest (no session map beside the capture); reversible scrub+rehydrate is a separate future path for live LLM prompts. Capture lives on the control plane; the Flutter client consumes it via catalog/ACP — do not treat client DevTools as the source of truth. New hops should ship with capture, not a follow-up.
- **Tests:** Prefer fakes/scripted providers; do not call a real LLM in unit tests.

Human setup and curl catalog examples: `README.md`.
