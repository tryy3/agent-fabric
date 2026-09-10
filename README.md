# Personal AI control plane

A hosted **agent control plane**: you configure agents in settings, then chat with them from Flutter, a TUI, or an IDE. Surfaces talk **ACP**. Model routing, MCP, memory, and sandboxes stay on the server.

This repository is in the **design phase**. Architecture and decisions live under [`docs/`](docs/architecture.md). Implementation has not started.

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
