# Personal AI control plane

A hosted **agent control plane**: you configure agents in settings, then chat with them from Flutter, a TUI, or an IDE. Surfaces talk **ACP**. Model routing, MCP, memory, and sandboxes stay on the server.

Architecture and decisions live under [`docs/`](docs/architecture.md).

## Layout

- [`controlplane/`](controlplane/) — Go control plane (ACP agent, WebSocket `/acp`, catalog REST `/v1`)
- [`client/`](client/) — Flutter web chat (ACP client over WebSocket)
- [`docs/`](docs/) — architecture and decisions

## Run (control plane + Flutter chat)

Requirements: Nix direnv shell (Go + Flutter) or local Go 1.22+ and Flutter 3.24+.

Agent definitions and provider credentials live in Postgres via `DATABASE_URL`; migrations run on startup. No `OPENAI_*` env vars or `-data-dir` JSON catalog are required. Any Postgres instance works (local Docker, managed cloud, etc.).

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
# optional: -addr :8080
```

### Configure catalog (Settings UI or curl)

Open **Settings → Providers** in the Flutter app, or use the catalog HTTP API on the same port:

**1. Create a provider**

```bash
curl -s localhost:8080/v1/providers -H 'content-type: application/json' \
  -d '{"name":"Unsloth","type":"openai_compatible","baseUrl":"http://127.0.0.1:8888/v1","apiKey":"sk-unsloth-…"}'
```

Note the returned `id` (e.g. `pr-abc123`).

**2. Refresh models** (required before creating an agent)

```bash
curl -s -X POST localhost:8080/v1/providers/PROVIDER_ID/models/refresh
```

**3. Create an agent** (pick a model id from the provider’s cached list)

```bash
curl -s localhost:8080/v1/agents -H 'content-type: application/json' \
  -d '{"name":"Coder","providerId":"PROVIDER_ID","defaultModel":"MODEL_ID"}'
```

Note the returned agent `id` (e.g. `ag-xyz789`).

```bash
curl -s localhost:8080/v1/threads -X POST -H 'content-type: application/json' -d '{}'
curl -s localhost:8080/v1/threads
```

### Chat in Flutter

```bash
cd client && flutter run -d chrome   # or -d linux / macos / windows
```

ACP WebSocket connectivity works on web and desktop/mobile via `web_socket_channel`; IO targets use protocol ping keepalive (30s) and a 30s connect timeout. The shell shows **Online**, **Reconnecting…**, or **Offline** — send is disabled while reconnecting/offline, but Settings and navigation stay available. Cleartext `ws://localhost:8080/acp` is the local-dev default only; use `wss://` in production.

Use the sidebar: **Settings** for providers/agents, **Chat** for the thread list and transcript.

- **+** starts an untitled thread. Pick an agent before sending.
- The first message titles the thread (first 8 words) unless you renamed it.
- Threads persist in Postgres; refresh restores the list and transcript.
- Thinking, the answer, and **Stats** are separate bubbles in arrival order. The caption (model · provider · tok/s) stays on the answer even if Stats is hidden. Thinking stays open while streaming and follows the Settings default after the turn. Defaults for Thinking/Stats live in **Settings → Chat**.

If the Flutter client shows `RpcError(-32603): Internal error`, check the controlplane log for `session/prompt failed` — that line has the real provider error (wrong key, no model loaded, bad base URL, etc.).

Design: [`docs/superpowers/specs/2026-09-12-dynamic-agents-providers-settings-design.md`](docs/superpowers/specs/2026-09-12-dynamic-agents-providers-settings-design.md).

Client tests:

```bash
cd client && flutter test
```

## Run (control plane + acp-cli)

Requirements: Nix direnv shell (provides Go) or a local Go 1.22+ toolchain.

Configure a provider and agent first (see above), then:

```bash
# terminal 1
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane

# terminal 2
go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -agent-id AGENT_ID -prompt "hello"
```

Tests (offline, fakes — no API keys; requires Nix `postgresql` on PATH via `nix develop`):

```bash
nix develop -c bash -lc 'go -C controlplane test ./...'
```

## Development (control plane)

### Sandbox FS tools (POC)

Standalone packages [`controlplane/internal/sandbox`](controlplane/internal/sandbox) and [`controlplane/internal/sandboxconfig`](controlplane/internal/sandboxconfig): local jailed FS and Docker/Podman exec-backed FS with `read_file` / `write_file` tools. **`cmd/controlplane` loads `./sandbox.json` from the process working directory at startup** (missing/invalid file → fatal). That file is **host engine defaults** only (`kind`, `runtime`, `binPath`, default image). Project-bound prompts open an isolated workspace (`agent-fabric.proj.{id}` volume at `/workspace` for Docker, `{dataDir}/projects/{projectId}/workspace` for local). Session scope still works for unbound ACP sessions or when an agent opts into `containerScope: session`. Relative `dockerfile` / mount `source` paths resolve against that directory.

Run the server from the directory that contains the file (e.g. `controlplane/` when using the docker example below):

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
```

Example `sandbox.json`:

```json
{"kind":"local","workspaceRoot":"/tmp/ws"}
```

```json
{
  "kind": "docker",
  "workspaceRoot": "/workspace",
  "docker": {
    "containerScope": "session",
    "idleTTLSeconds": 600,
    "runtime": "auto",
    "image": "alpine:3.20",
    "mounts": [{ "source": "./data", "target": "/workspace", "readOnly": false }]
  }
}
```

The docker example mounts [`controlplane/data/`](controlplane/data/) (e.g. `test.json`) at `/workspace` inside the container.

#### End-to-end smoke (Flutter)

After providers and agents are configured (see above), start the control plane from a directory with `sandbox.json` (`go -C controlplane run ./cmd/controlplane` loads [`controlplane/sandbox.json`](controlplane/sandbox.json)), then run Flutter (`cd client && flutter run -d chrome`). Pick an agent, open a thread, and prompt e.g. **“Read test.json from the workspace and summarize it.”**

- The model should call **`read_file`**. The transcript shows a collapsible activity bubble (same pattern as **Thinking**) titled **Read file**, with **Input** (path) and **Output** (file contents). It stays expanded while the call is in progress, then collapses when idle.
- Tool calls persist in the thread — refresh restores the same bubbles in order (thought / tool / message / stats).
- Sandbox file tools use **`sandbox.json` as host engine defaults** (runtime, image, binary). Each project gets its own workspace; there is no tool picker in **Settings** yet.

Unit tests:

```bash
go -C controlplane test ./internal/sandbox/... ./internal/sandboxconfig/...
```

Integration (requires Docker or Podman):

```bash
go -C controlplane test -tags=integration ./internal/sandbox/docker/
```

## Read first

1. [Architecture](docs/architecture.md) — layers, flows, what belongs where
2. [Decisions](docs/decisions.md) — what we chose and why

## Intent

Most harnesses (Hermes, editor-native agents, CopilotKit-style apps) collapse UI, agent loop, and inference into one process, or they let the client send tools, model, and transcript on every run. That gets brittle: every surface reimplements policy, and memory diverges.

Here:

- **Catalog API** (ours) — create and edit agent definitions: model, MCP, sandbox, memory, policy
- **ACP v1** — how a client prompts a chosen definition
- **Inference** — behind the definition (`scripted` in tests, a real provider in production)

IDEs can use the same agents as the phone. Docker never pretends to be ACP `fs/*`.
