# Agent Fabric Flutter client

The first-party Agent Fabric cockpit. It connects to the control plane's ACP WebSocket for chat and its catalog HTTP API for configuration and project workspaces.

## What it provides

- Project and thread navigation with project-scoped workspace state.
- Dockable **Threads**, **Files**, and **Chat** panes.
- File editing plus web, image, audio, and download document views.
- Catalog settings for providers, agents, projects, resources, environments, integrations, and display preferences.
- ACP streaming, permissions, user questions, tool activity, hop-capture inspection, and reconnect status.

The client is a cockpit: it does not own the provider loop, provider credentials, canonical transcript, sandbox, or MCP execution. Plane-hosted MCP execution is planned; stored project MCP settings are not executed yet.

## Run locally

Start Postgres and the control plane from the repository root first. The control plane process must start with `controlplane/sandbox.json` as its working-directory configuration.

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane

cd client
flutter run -d chrome
```

The default local endpoints are `http://localhost:8080` for the catalog and `ws://localhost:8080/acp` for ACP. See the repository [README](../README.md) for provider and agent setup.

## Verify

CI uses Flutter 3.47.0 and runs these gates:

```bash
dart format --output=none --set-exit-if-changed .
flutter analyze --fatal-infos
flutter test --test-randomize-ordering-seed random
```
