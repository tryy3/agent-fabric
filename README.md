# Personal AI control plane

A hosted **agent control plane**: you configure agents in settings, then chat with them from Flutter, a TUI, or an IDE. Surfaces talk **ACP**. Model routing, MCP, memory, and sandboxes stay on the server.

Architecture and decisions live under [`docs/`](docs/architecture.md).

## Run (control plane echo slice)

Requirements: Nix direnv shell (provides Go) or a local Go 1.22+ toolchain.

```bash
# terminal 1
go run ./cmd/controlplane

# terminal 2
go run ./cmd/acp-cli -addr localhost:8080 -prompt "hello"
```

Tests (offline, no API keys):

```bash
go test ./...
```

Design: [`docs/superpowers/specs/2026-09-11-controlplane-acp-echo-design.md`](docs/superpowers/specs/2026-09-11-controlplane-acp-echo-design.md).

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
