# Decisions

Record of what we chose while planning. Newest last. Status is `accepted` unless noted.

---

## 1. Control plane, not a chat wrapper

**Status:** accepted

The product is a hosted control plane. Sessions, memory, model routing, MCP, and sandboxes live on the server. Flutter, TUI, and IDEs are cockpits.

**Why:** Hermes-like and editor-native harnesses couple UI, tools, and memory in one process. That is what felt buggy. Loosely coupled surfaces only work if policy is not reimplemented in each client.

---

## 2. ACP is the runtime protocol

**Status:** accepted

Once an agent definition exists, clients talk to it with **ACP** (Agent Client Protocol): `initialize`, `session/new`, `session/prompt`, `session/update`, `session/request_permission`, cancel, optional `configOptions`.

**Why:** ACP already models a rich agent cockpit (streaming, tool UX, permissions, plans) and is what IDEs implement. Using it as the runtime means Zed and Flutter can share the same logical agents.

ACP does **not** configure agents. It assumes they exist.

---

## 3. Catalog API is ours; not ACP

**Status:** accepted

Creating/editing agents (model, MCP, sandbox, capabilities, memory) is an internal HTTP API used by settings. We then **expose** each definition as a logical ACP agent.

The public ACP Registry is a marketplace of implementations (Claude Code, Gemini CLI). It is not our personal “Work / Personal / Research” list. Clients need `GET /v1/agents` (or equivalent) before opening ACP.

**Hot reload:** definitions are versioned. A session pins the snapshot from `session/new`. Settings changes apply to the next session. `initialize` capabilities stay those of the connection until reconnect.

---

## 4. ACP v1 on the wire; internals aimed at v2

**Status:** accepted

Ship **ACP v1**. Design the runtime as if the client were not the computer (v2’s direction). Add a v2 adapter when the draft stabilizes; do not start v2-only.

**Why:** v2 has been draft since 20 July 2026. Maintainers say it will change, must be feature-flagged, and v1-only peers remain common. Official SDKs treat v2 as experimental. Flutter/Dart and current IDEs are v1.

v1 already allows our execution model: advertise `fs` / `terminal` unsupported, run Docker ourselves, use `configOptions` for advertised knobs.

**Later:** v2’s prompt lifecycle (ack vs idle, background updates, several observers) is worth adopting via a second codec, not a rewrite of the catalog.

---

## 5. Inference is behind the agent

**Status:** accepted

ACP has no `llm/complete`. The client sees an agent and its capabilities, not a vendor. A definition’s provider may be scripted (tests) or a real API. Optional `configOptions` with category `model` may *display* or constrain models; the plane can refuse or omit the picker.

---

## 6. Do not use AG-UI as the application API

**Status:** accepted

AG-UI is a streaming UX codec (`RunAgentInput`: messages, tools, state). Quickstarts put tools and transcript on the client because they assume the app owns the session.

That inverts this project. We may emit AG-UI later for a specific widget. We will not let Flutter send backend tools, MCP, or history as the source of truth.

---

## 7. Execution origins: sandbox, MCP, client

**Status:** accepted

Tool execution is routed by origin:

- **sandbox** — control plane Docker (or in-process)
- **mcp** — plane-hosted MCP
- **client** — round-trip to the surface (clipboard, localStorage, IDE buffers)

ACP v1 `fs/*` always means “ask the client.” It is **not** Docker. Overloading `fs/*` for both localStorage and Docker is how agents end up with two file implementations; ACP v2 is removing client fs/terminal for that reason.

Client-local tools should be MCP-over-ACP when that RFD ships, or a `_client/*` / device MCP until then — not a fake editor filesystem unless the surface *is* an editor.

---

## 8. Client contract (what a cockpit may send)

**Status:** accepted

Allowed: user prompt, cancel, permission replies, client-only context (open files, UI surface), client-origin tool *results*, optional hints the server may ignore.

Forbidden as policy (ignored if present): model/provider bypass, MCP secrets, system prompt, canonical transcript, backend tool definitions.

The plane advertises capabilities and `configOptions`. Changing policy is the catalog API, not a chat field.

---

## 9. Memory is server-side and scoped

**Status:** accepted

Scopes: working (transcript), session, project, long-term. Clients inspect; the runtime hydrates. Start without a vector database.

---

## 10. Tests use a scripted provider

**Status:** accepted

The LLM is an interface. CI and local default: deterministic scripted model. No network required to test routing, memory isolation, definition pinning, or “client cannot inject tools.”

---

## 11. Flutter is the first-party client

**Status:** accepted

Flutter covers web, mobile, and desktop. A TUI can be added as another ACP client. We do not need a second product protocol for first-party chat if ACP (over WebSocket or equivalent) is viable; remote ACP HTTP is still a draft, so the **transport** may be pragmatic while the **messages** stay ACP.

---

## Explicitly deferred

- ACP v2 as default wire format
- MCP-over-ACP (use a stopgap for device tools if needed)
- Vector memory
- Multi-user auth product
- Implementing Docker in the first vertical slice (the *slot* exists on the definition; the worker can come after a scripted in-process agent)
- Naming the product
