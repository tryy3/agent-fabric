# Personal AI control plane

A hosted **agent control plane**: you configure agents in settings, then chat with them from Flutter, a TUI, or an IDE. Surfaces talk **ACP**. Model routing, MCP, memory, and sandboxes stay on the server.

Architecture and decisions live under [`docs/`](docs/architecture.md).

## Layout

- [`controlplane/`](controlplane/) — Go control plane (ACP agent, WebSocket `/acp`)
- [`client/`](client/) — Flutter web chat (ACP client over WebSocket)
- [`docs/`](docs/) — architecture and decisions

## Run (control plane + Flutter chat)

Requirements: Nix direnv shell (Go + Flutter) or local Go 1.22+ and Flutter 3.24+. **Unsloth Studio** (or any OpenAI-compatible endpoint) must be running.

Set required env vars (the control plane exits at startup if any are missing):

```bash
export OPENAI_BASE_URL=http://127.0.0.1:<unsloth-port>/v1
export OPENAI_API_KEY=sk-local
export OPENAI_MODEL=<model-id>

go -C controlplane run ./cmd/controlplane
```

```bash
# terminal 2
cd client && flutter run -d chrome
```

Send a message in the browser; the agent streams a model reply (requires Unsloth/OpenAI-compatible endpoint).

Design: [`docs/superpowers/specs/2026-09-12-controlplane-openai-inference-design.md`](docs/superpowers/specs/2026-09-12-controlplane-openai-inference-design.md).

Client tests:

```bash
cd client && flutter test
```

## Run (control plane + acp-cli)

Requirements: Nix direnv shell (provides Go) or a local Go 1.22+ toolchain. Same OpenAI env vars as above.

```bash
# terminal 1
go -C controlplane run ./cmd/controlplane

# terminal 2
go -C controlplane run ./cmd/acp-cli -addr localhost:8080 -prompt "hello"
```

Tests (offline, fakes — no API keys):

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
