# Control plane ACP echo slice

**Date:** 2026-09-11  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)

## Problem

The repository has architecture and decisions but no runnable control plane. We need a first vertical slice that proves **remote ACP v1** works end-to-end with a scripted agent, before catalog, Flutter, memory, MCP, or real inference.

## Goals

- Scaffold a Go control-plane module with Nix tooling sufficient to build and test.
- Expose one hardcoded logical agent over **WebSocket JSON-RPC** on `/acp`.
- Implement ACP Agent role with `github.com/coder/acp-go-sdk`.
- Scripted **echo** provider: `session/prompt` streams the user text back as assistant `session/update`s (no network, no API keys).
- Prove the path with an integration test and a small `acp-cli` for manual smoke.

## Non-goals

- Catalog HTTP API
- Auth / multi-user
- Memory, MCP, sandbox/Docker
- Real LLM providers
- Flutter client
- Stdio ACP transport (keep the design open; do not implement in this slice)
- ACP v2 wire format

## Approach

**Chosen:** `coder/acp-go-sdk` for typed ACP Agent/Client + JSON-RPC, plus a thin WebSocket bridge we own (aligned with the remote ACP RFD: upgrade on `/acp`).

**Rejected for this slice:**

- `eino-contrib/acp` — has WS built-in but pulls Hertz and ties us to that ecosystem early.
- Hand-rolled JSON-RPC only — reinvent protocol edge cases and slows growth toward a real agent.

## Architecture

```text
acp-cli / tests          controlplane process
───────────────          ────────────────────
  WebSocket  ──────────►  /acp upgrade
                          │
                          ▼
                     transport/ws  (byte duplex ↔ SDK)
                          │
                          ▼
                     coder/acp-go-sdk (JSON-RPC)
                          │
                          ▼
                     internal/agent (ACP Agent)
                          │
                          ▼
                     runtime (sessions + pinned echo definition)
                          │
                          ▼
                     echo provider (stream text updates)
```

### Components

| Piece | Responsibility |
| --- | --- |
| `cmd/controlplane` | HTTP listener (default `:8080`), WS on `/acp`, wire agent + runtime |
| `cmd/acp-cli` | Dial `ws://…/acp`, run initialize → session/new → session/prompt, print updates |
| `internal/agent` | SDK `Agent` implementation; minimal capabilities (no fs/terminal; no tools yet) |
| `internal/runtime` | In-memory session map; `session/new` pins the hardcoded `echo` definition snapshot |
| `internal/transport/ws` | HTTP upgrade → WebSocket; adapt read/write for the SDK connection |
| Echo provider | Deterministic: stream user prompt text back as assistant message chunks |

### Runtime flow

1. Client opens WebSocket to `/acp`.
2. `initialize` — negotiate ACP v1; advertise minimal agent capabilities.
3. `session/new` — allocate session id; pin `echo` definition snapshot.
4. `session/prompt` — extract user text; stream `session/update` chunks; complete the turn.
5. Further prompts reuse the session until disconnect.

### Errors and lifecycle

- Unknown session or malformed JSON-RPC → protocol errors on the connection.
- WebSocket close mid-prompt → cancel in-flight turn; drop connection-local session state (no persistence in this slice).

## Repo layout

```text
cmd/controlplane/
cmd/acp-cli/
internal/agent/
internal/transport/ws/
internal/runtime/
flake.nix                 # add Go toolchain (+ gopls/gotools as needed)
go.mod
```

Go module path: match repo identity at init (e.g. derived from the git remote / `tryy3/agent-fabric`). Adjustable if the published module path differs.

## Tooling

- Nix flake `devShell`: `go`, and editor/support tools needed to `go test` / `go run` this slice (`gopls` or `gotools`).
- Flutter and frontend packages stay out of the flake until that work starts.
- No new Cursor skills required for this slice. Optional later: AGENTS.md / skill documenting how to run server + CLI.

## Verification

**Manual**

```text
go run ./cmd/controlplane
go run ./cmd/acp-cli -addr localhost:8080 -prompt "hello"
# streams echoed user text via session/update
```

**Automated**

- Unit: echo provider produces expected update chunks.
- Integration: dial WebSocket against an in-process (or `httptest`) server; assert initialize → session/new → session/prompt → echoed updates.
- `go test ./...` with no network or API keys.

## Success criteria

1. Go module builds in the Nix shell.
2. WebSocket ACP path works: initialize → session/new → session/prompt.
3. Echo streams back via `session/update`.
4. Integration test passes offline.
5. README or a short docs note explains how to run server + CLI.

## Follow-ups (explicitly later)

- Catalog API (`GET /v1/agents`, CRUD)
- Stdio transport adapter for IDE spawn
- Scripted playbooks (tool calls, permissions)
- Real providers behind the same Agent interface
- Flutter cockpit over the same `/acp` endpoint
