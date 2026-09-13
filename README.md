# Personal AI control plane

A hosted **agent control plane**: you configure agents in settings, then chat with them from Flutter, a TUI, or an IDE. Surfaces talk **ACP**. Model routing, MCP, memory, and sandboxes stay on the server.

Architecture and decisions live under [`docs/`](docs/architecture.md).

## Layout

- [`controlplane/`](controlplane/) — Go control plane (ACP agent, WebSocket `/acp`, catalog REST `/v1`)
- [`client/`](client/) — Flutter web chat (ACP client over WebSocket)
- [`docs/`](docs/) — architecture and decisions

## Run (control plane + Flutter chat)

Requirements: Nix direnv shell (Go + Flutter) or local Go 1.22+ and Flutter 3.24+.

Agent definitions and provider credentials live in Postgres. Set `DATABASE_URL` (any Postgres instance works). Migrations run on startup. No `OPENAI_*` env vars or `-data-dir` JSON catalog are required.

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
cd client && flutter run -d chrome
```

Use the sidebar: **Settings** for providers/agents, **Chat** for the thread list and transcript.

- **+** starts an untitled thread. Pick an agent before sending.
- The first message titles the thread (first 8 words) unless you renamed it.
- Threads persist in Postgres; refresh restores the list and transcript.

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
# terminal 1 (DATABASE_URL required; see above)
go -C controlplane run ./cmd/controlplane

# terminal 2
go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -agent-id AGENT_ID -prompt "hello"
```

Tests (catalog tests need Docker or Podman; ACP tests use fakes — no API keys):

```bash
go -C controlplane test ./...
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
