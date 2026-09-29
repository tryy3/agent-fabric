# controlplane

Go module for the Agent Fabric control plane: ACP WebSocket agent (`/acp`), catalog REST (`/v1`), Postgres-backed providers/agents/projects/threads, project workspace routes, provider adapters, and sandbox tools.

The process requires `config.json` in its working directory and runs migrations at startup. Use Go 1.26+; full local setup, catalog examples, and verification commands are in the [repository README](../README.md).

Implemented provider types are `openai_compatible`, `unsloth_studio`, `opencode_zen`, and `opencode_go`. Plane-hosted MCP execution and scoped memory are deferred; project MCP configuration is stored but not executed.

Module path: `github.com/tryy3/agent-fabric` (this directory is the module root).

Operator setup and catalog curl examples live in the [repository README](../README.md). Agent commands and constraints: [AGENTS.md](AGENTS.md).
