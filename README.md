# Personal AI control plane

A hosted **agent control plane** with a Flutter workbench: you configure assistants, projects, inference connections, and execution environments in the catalog, then chat with an assistant over ACP. Model routing, tool execution, canonical thread history, and sandboxes stay on the server. Plane-hosted MCP execution and scoped memory are planned, not yet implemented.

Architecture and decisions live under [`docs/`](docs/architecture.md).

## License

Agent Fabric is free software licensed under the [GNU Affero General Public
License v3.0 only](LICENSE) (`AGPL-3.0-only`). You may use, modify, and
redistribute it, including commercially, under that license's terms. Modified
versions offered to users over a network must provide those users access to the
corresponding source.

Copyright (C) 2026 Agent Fabric contributors.

Published container images include the project license, source-location
information, and third-party notices. Their OCI metadata identifies the exact
source revision, and GHCR releases include signed provenance and an SBOM
attestation.

## Layout

- [`controlplane/`](controlplane/) — Go control plane (ACP agent, WebSocket `/acp`, catalog REST `/v1`)
- [`client/`](client/) — Flutter project-scoped workbench (ACP client over WebSocket)
- [`docs/`](docs/) — architecture and decisions
- [`deploy/`](deploy/) — compose example for GHCR images (`controlplane` + `client-web`)

## Deploy with Docker

On pushes to `main` and `v*` tags, CI builds multi-arch images and pushes them to GHCR (`…/controlplane`, `…/client-web`). See [`deploy/README.md`](deploy/README.md) for `docker compose -f deploy/compose.yaml up -d`, Tailscale Serve, and runtime URL env vars (`CATALOG_BASE`, `ACP_URI`, `CONTROLPLANE_UPSTREAM`).

## Run (control plane + Flutter workbench)

Requirements: Nix direnv shell (Go + Flutter) or local Go 1.26+ and Flutter 3.47.0+.

Agent definitions and provider credentials live in Postgres via `DATABASE_URL`; migrations run on startup. No `OPENAI_*` env vars or `-data-dir` JSON catalog are required. Any Postgres instance works (local Docker, managed cloud, etc.).

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
# optional: -addr :8080
```

### Configure catalog (Settings UI or curl)

Open **Settings → Connections** in the Flutter app, or use the catalog HTTP API on the same port:

**1. Create an inference connection**

```bash
curl -s localhost:8080/v1/inference/connections -H 'content-type: application/json' \
  -d '{"name":"Unsloth","type":"unsloth_studio","baseUrl":"http://127.0.0.1:8888/v1","apiKey":"sk-unsloth-…"}'
```

Note the returned `id` (e.g. `pr-abc123`).

**2. Refresh models** (required before creating an assistant)

```bash
curl -s -X POST localhost:8080/v1/inference/connections/CONNECTION_ID/models/refresh
```

**3. Create an assistant** (pick a model id from the connection’s cached list)

```bash
curl -s localhost:8080/v1/assistants -H 'content-type: application/json' \
  -d '{"name":"Coder","inferenceConnectionId":"CONNECTION_ID","defaultModel":"MODEL_ID"}'
```

Note the returned assistant `id` (e.g. `as-xyz789`).

```bash
curl -s localhost:8080/v1/threads -X POST -H 'content-type: application/json' -d '{}'
curl -s localhost:8080/v1/threads
```

### Chat in Flutter

```bash
cd client && flutter run -d chrome   # or -d linux / macos / windows
```

ACP WebSocket connectivity works on web and desktop/mobile via `web_socket_channel`; IO targets use protocol ping keepalive (30s) and a 30s connect timeout. The shell shows **Online**, **Reconnecting…**, or **Offline** — send is disabled while reconnecting/offline, but Settings and navigation stay available. Cleartext `ws://localhost:8080/acp` is the local-dev default only; use `wss://` in production.

Use the sidebar for projects, threads, and **Settings**. Each active project has a dockable workbench with Project files, Threads, and Chat. Files can open an editor, web preview, image preview, audio preview, or download view; project filesystem routes also support Git history, checkpoints, restore/diff, and export.

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

Requirements: Nix direnv shell (provides Go) or a local Go 1.26+ toolchain.

Configure a provider and agent first (see above), then:

```bash
# terminal 1
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane

# terminal 2
go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -assistant-id AGENT_ID -prompt "hello"
```

Tests (offline, fakes — no API keys; requires Nix `postgresql` on PATH via `nix develop`):

```bash
nix develop -c bash -lc 'go -C controlplane test ./...'
```

## Development (control plane)

### Sandbox FS tools (POC)

Standalone package [`controlplane/internal/sandbox`](controlplane/internal/sandbox) plus host boot config [`controlplane/internal/engineconfig`](controlplane/internal/engineconfig): local jailed FS and Docker/Podman exec-backed FS with `read_file` / `write_file` tools. **`cmd/controlplane` loads `./config.json` from the process working directory at startup** (missing/invalid file → fatal). That file is **host engine config** only (`databaseUrl`, `listenAddr`, `dataDir`, docker `runtime` / `binPath` / optional `identityPrefix`). With `"runtime": "auto"` and an empty `binPath`, the plane prefers `podman` on `PATH`, then `docker` — pin either with `"runtime": "podman"|"docker"` and/or an absolute `binPath` when you need a fixed binary. Image, kind, project root, idle TTL, and container name template live in catalog **Settings → Environment** (`GET/PATCH /v1/settings`) and apply on the next prompt. Docker containers are addressed with `--name` from the template (`{projectID}`, `{threadID}`, `{random}`); two projects that set the same static name reuse one container, and a running name with a different image or mounts fails instead of recreating. `DATABASE_URL` and `-addr` still win when set. Project-bound prompts still mount the Phase 1 workspace volume (`agent-fabric.proj.{id}` at `/workspace` for Docker, `{dataDir}/projects/{projectId}/workspace` for local).

Run the server from the directory that contains the file (e.g. `controlplane/` when using the docker example below):

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
```

Example `config.json`:

```json
{
  "databaseUrl": "postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable",
  "listenAddr": ":8080",
  "dataDir": "./data",
  "docker": {
    "runtime": "auto",
    "binPath": "",
    "identityPrefix": ""
  }
}
```

Deprecated overlay keys still present in an old file (`kind`, `projectRoot`, `image`, `idleTTLSeconds`) are copied into `plane_settings` once on first boot, then ignored.

#### End-to-end smoke (Flutter)

After providers and agents are configured (see above), start the control plane from a directory with `config.json` (`go -C controlplane run ./cmd/controlplane` loads [`controlplane/config.json`](controlplane/config.json)), then run Flutter (`cd client && flutter run -d chrome`). Pick an agent, open a thread, and prompt e.g. **“Read test.json from the workspace and summarize it.”** Global image/kind changes in **Settings → Environment** apply on the next prompt.

- The model should call **`read_file`**. The transcript shows a collapsible activity bubble (same pattern as **Thinking**) titled **Read file**, with **Input** (path) and **Output** (file contents). It stays expanded while the call is in progress, then collapses when idle.
- Tool calls persist in the thread — refresh restores the same bubbles in order (thought / tool / message / stats).
- Sandbox file tools use **catalog overlay settings** for image/kind (engine file only supplies host process knobs). Each project gets its own workspace.

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

- **Catalog API** (ours) — create and edit agents, providers, projects, sandbox/environment settings, and policy metadata
- **ACP v1** — how a client prompts a chosen definition
- **Inference** — behind the definition (real provider adapters in production; injected fakes in tests)

Docker never pretends to be ACP `fs/*`. Plane-hosted MCP execution and scoped memory remain planned work; see [#62](https://github.com/tryy3/agent-fabric/issues/62) for MCP.
