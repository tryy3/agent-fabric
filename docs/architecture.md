# Architecture

This is the system we are building. Decisions that led here are in [decisions.md](decisions.md).

## Goal

A **central control plane** you can chat with from web, mobile, desktop, TUI, and IDEs, with:

- Long-term, session, and project memory on the server
- Per-agent configuration (model, MCP, sandbox, capabilities)
- Isolated execution (Docker) when an agent needs a computer
- Tests that do not call a real LLM

Clients are replaceable cockpits. They do not own the agent.

## Layers

```text
┌─────────────────────────────────────────────────────────────┐
│  Surfaces                                                     │
│  Flutter (web / app / desktop)  ·  TUI  ·  IDE (Zed, …)     │
└──────────────┬───────────────────────────────┬──────────────┘
               │ catalog API (settings)         │ ACP v1 (runtime)
               │ list / create / edit agents      │ session/prompt …
               ▼                                 ▼
┌─────────────────────────────────────────────────────────────┐
│  Control plane                                                │
│  agent catalog · sessions · memory · MCP host · sandboxes    │
│  ACP agent role (one logical agent per definition)            │
└──────────────┬───────────────────────────────┬──────────────┘
               │ MCP                             │ provider API
               ▼                                 ▼
         tools / data                    OpenAI / local / scripted
```

ACP names two peers: **Client** and **Agent**. Inference is not a protocol actor. The control plane *implements* the ACP Agent role for each configured definition.

## Two APIs

| API | Audience | Job |
| --- | --- | --- |
| **Catalog** (our HTTP API) | Settings UI, admin | Create/update agent definitions: model, MCP, sandbox, memory, who may use them |
| **ACP v1** | Chat UI, TUI, IDEs | Talk to an *already configured* agent |

ACP has no “create an agent with this model.” It assumes the agent exists. The catalog is how agents exist. After the user picks `work`, the client opens ACP against that agent and the rest is stock ACP (`initialize` → `session/new` → `session/prompt`).

Switching agent is a **new session on a different definition**, not a field on the current turn. Small knobs *inside* a definition (ask vs auto, optional model set) can be ACP `configOptions`.

## Agent definitions

A definition is internal config, versioned, hot-reloadable:

- Identity: id, name, description
- Provider: which inference backend and default model
- Allowed `configOptions` (optional model list, mode, …)
- MCP servers the **plane** attaches (not the Flutter app)
- Sandbox profile: none / Docker image / resource limits
- Tools and permission policy
- Memory scopes this agent may read/write
- Prompt / policy text

At `session/new`, the runtime **pins a snapshot** of the definition. In-flight turns do not mutate when you edit settings. The next session picks up the new version.

`initialize` capabilities come from that snapshot. Changing advertised capabilities requires a new connection (ACP negotiates capabilities once per connection).

One OS process can host many **logical** ACP agents. Isolation is a property of the definition (in-process vs Docker), not “one subprocess per agent” unless we choose that later.

## Runtime path

When a client sends `session/prompt` to agent `work`:

1. Resolve the session’s pinned definition
2. Hydrate memory for that definition’s scopes
3. Attach that definition’s MCP and sandbox
4. Call that definition’s provider (`scripted` or a real LLM)
5. Execute tools by **origin** (see below)
6. Stream ACP `session/update` (text, tool calls, plans, permissions)

The client never sends model, backend tools, MCP secrets, system prompt, or the canonical transcript. If an IDE still sends `cwd` / `mcpServers` on `session/new`, the plane **overrides from the definition**, except true **client-origin** tools (device MCP or `_` extension methods).

## Execution surfaces

Where work runs is a runtime concern, not “whatever ACP `fs/*` means.”

| Origin | Examples | Runs |
| --- | --- | --- |
| `sandbox` | files, shell, code exec | Docker (or none) on the control plane |
| `mcp` | GitHub, search, user-configured servers | MCP host on the control plane |
| `client` | clipboard, localStorage, IDE buffers | the connected surface, round-trip |

A phone advertises client tools like clipboard and **no** host filesystem. A TUI may advertise real host fs/terminal *as client-origin tools* (or v1 `fs/*` / `terminal/*` if we ever enable them for that surface). Docker is always `sandbox`, never ACP `fs/*`.

ACP v1 *can* call `fs/*` on a client that advertised it. We do not map that to Docker. v2 is removing client fs/terminal from the protocol; we already behave as if that were true.

## Surfaces

**Flutter** is the first-party cockpit (web, iOS, Android, desktop): session list, transcript, permissions, memory inspector, settings against the catalog. It is not an agent framework.

**TUI** is optional and closer to an IDE (real cwd, maybe host tools). It should speak ACP against the same agents.

**IDEs** are ACP clients against the same logical agents. File buffers may round-trip to the editor; isolated execution still goes to the plane’s sandbox.

A first-party **thin session API** is not required if Flutter speaks ACP. We still own the catalog HTTP API. If remote ACP (HTTP/WebSocket) is too rough for Flutter web, a thin transport that carries the same ACP JSON-RPC (or a WebSocket) is an implementation detail, not a second product protocol.

## Memory

Memory lives on the plane. Clients inspect it; they do not implement it.

| Scope | Lifetime | Injected |
| --- | --- | --- |
| Working | this turn | recent transcript |
| Session | this thread | every turn in the session |
| Project | this workspace / repo | every turn in that project |
| Long-term | across projects | profile always-on; rest retrieved |

Start with keyed records and search (FTS is enough). A vector index is optional later.

## Inference

The provider is an interface. Default for development and tests: **scripted** (deterministic tool calls, streamed tokens, no network). Production definitions point at OpenAI-compatible or local servers.

From ACP’s point of view, scripted and GPT are the same agent. The client sees capabilities and optional `configOptions`, not a vendor SDK.

## Protocol map (what we are *not* using as the spine)

| Protocol | Role here |
| --- | --- |
| **ACP v1** | Runtime: client ↔ logical agent |
| **MCP** | Agent ↔ tools/data; plane is the host |
| **Catalog HTTP** | Settings / definitions |
| AG-UI | Not the application API. Optional later as a codec if some widget needs it |
| ACP v2 | Direction for internals; second wire adapter when it stabilizes |
| A2A | Not needed for v1 of this product |

Remote ACP HTTP/WebSocket is still a draft (separate from v1 vs v2). Stdio is the stable ACP transport (IDE spawns a process). Our hosted case needs a remote transport; WebSocket is the realistic Flutter path until the RFD settles.

## What we are not building first

- Full ACP v2 wire support
- Inventing a competing token-stream protocol
- Client-owned MCP/model config on each chat request
- Vector DB
- Auth/identity product (can stay local/single-user until needed)
- Multi-tenant SaaS

## Testing stance

Assert against the **internal runtime** (definition pin, memory scopes, provider routing, “client cannot inject backend tools”). ACP encoding is tested with fixtures. The scripted provider means CI never needs API keys.
