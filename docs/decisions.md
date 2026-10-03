# Decisions

Record of what we chose while planning. Newest last. Status is `accepted` unless noted.

---

## 1. Control plane, not a chat wrapper

**Status:** accepted

The product is a hosted control plane. Sessions, model routing, canonical thread history, and sandboxes live on the server. Scoped memory and plane-hosted MCP execution are planned server-side capabilities. Flutter is the implemented cockpit; TUI and IDE surfaces remain future clients.

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

Creating/editing agents (model, inference, sandbox, capabilities, and future-facing MCP/memory metadata) is an internal HTTP API used by settings. We then **expose** each definition as a logical ACP agent.

The public ACP Registry is a marketplace of implementations (Claude Code, Gemini CLI). It is not our personal “Work / Personal / Research” list. Clients need `GET /v1/assistants` (or equivalent) before opening ACP.

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

ACP has no `llm/complete`. The client sees an agent and its capabilities, not a vendor. Live definitions use catalog provider adapters; tests inject fakes. Optional `configOptions` with category `model` may *display* or constrain models; the plane can refuse or omit the picker.

---

## 6. Do not use AG-UI as the application API

**Status:** accepted

AG-UI is a streaming UX codec (`RunAgentInput`: messages, tools, state). Quickstarts put tools and transcript on the client because they assume the app owns the session.

That inverts this project. We may emit AG-UI later for a specific widget. We will not let Flutter send backend tools, MCP, or history as the source of truth.

---

## 7. Execution origins: sandbox, MCP, client

**Status:** accepted

Tool execution will be routed by origin:

- **sandbox** — control plane Docker (or in-process)
- **mcp** — planned plane-hosted MCP
- **client** — planned round-trip to the surface (clipboard, localStorage, IDE buffers)

Today, only environment-origin tools are wired into the agent loop. MCP execution is tracked in [#62](https://github.com/tryy3/agent-fabric/issues/62).

ACP v1 `fs/*` always means “ask the client.” It is **not** Docker. Overloading `fs/*` for both localStorage and Docker is how agents end up with two file implementations; ACP v2 is removing client fs/terminal for that reason.

Client-local tools should be MCP-over-ACP when that RFD ships, or a `_client/*` / device MCP until then — not a fake editor filesystem unless the surface *is* an editor.

---

## 8. Client contract (what a cockpit may send)

**Status:** accepted

Allowed: user prompt, cancel, permission replies, client-only context (open files, UI surface), client-origin tool *results*, optional hints the server may ignore.

Forbidden as policy (ignored if present): model/provider bypass, MCP secrets, effective instructions (system prompt), canonical transcript, backend tool definitions.

The plane advertises capabilities and `configOptions`. Changing policy is the catalog API, not a chat field.

---

## 9. Memory will be server-side and scoped

**Status:** accepted

Today the runtime replays persisted thread messages and reasoning parts only. Scopes such as working, session, project, and long-term are the intended design; clients will inspect them and the runtime will hydrate them when implemented. Start without a vector database.

---

## 10. Tests use injected fake providers

**Status:** accepted

The LLM is an interface. CI uses deterministic injected fake streamers, with no network required to test routing, definition pinning, or “client cannot inject tools.” The live provider registry has no scripted catalog type.

---

## 11. Flutter is the first-party client

**Status:** accepted

Flutter covers web, mobile, and desktop. A TUI can be added as another ACP client. We do not need a second product protocol for first-party chat if ACP (over WebSocket or equivalent) is viable; remote ACP HTTP is still a draft, so the **transport** may be pragmatic while the **messages** stay ACP.

---

## 12. OpenCode Zen/Go are first-class provider types

**Status:** accepted

Catalog provider `type` includes `openai_compatible` (Custom: user base URL + key), `unsloth_studio`, `berget_ai`, `opencode_zen`, and `opencode_go`. OpenCode and Berget types fix the official base URL and require only an API key. At prompt time the plane picks Chat Completions, Anthropic Messages, or OpenAI Responses from the model id (Hermes-style prefix table) and sends `User-Agent: agent-fabric/…` plus a stable `x-opencode-session` derived from the ACP session id. Gemini and Jev models are filtered from OpenCode model refresh until adapters exist. Field matrix and provider notes: [inference-providers.md](inference-providers.md).

**Why:** OpenCode’s gateways mix wire APIs per model; treating them as a single Chat Completions base URL breaks Claude/GPT/Grok paths. Separate Zen vs Go types match distinct billing and model catalogs. Berget is a first-class EU Chat Completions endpoint with sampler and usage extras.

---

## 13. Tool Gate vs ask_user

**Status:** accepted

Before sandbox tools run, a pluggable **Gate** (`Evaluator` chain) returns `allow`, `ask`, or `deny`. Hardcoded rules ship first; classifier models can append later without changing the agent loop. `ask` uses ACP `session/request_permission` (Allow once / Allow for this session / Reject). `deny` fails the tool with no prompt.

Clarification is a separate plane-owned **`ask_user`** tool that uses ACP `elicitation/create` (form). Clients render permission and clarification with distinct UX (high-attention vs calm). Policy stays on the plane; clients only present options and reply.

**Why:** Security authorization and product questions must not share one dialog. Gate decisions fail closed even when the model never asks; ask_user is model-initiated preference gathering.

---

## 14. Agent inference settings (not ACP sampling)

**Status:** accepted

Generation knobs live on the agent as `settings.inference` (catalog PATCH), snapshotted into the session pin at `session/new`. Adapters send only set fields (`omitempty`). Provider types expose a shared core (`temperature`, max tokens, reasoning effort) plus type-specific extras (see [inference-providers.md](inference-providers.md)). Mid-chat ACP `configOptions` for sampling are deferred.

**Why:** Keeps client boundary and hot-reload rules consistent with provider/model pinning; local Unsloth and cloud Berget need deep knobs without forcing SillyTavern-style ACP panels on every cloud model.

---

## 15. Latest-prompt retry is soft-supersede (v1)

**Status:** accepted

Retry of the **latest completed user prompt** ([#54](https://github.com/tryy3/agent-fabric/issues/54) v1 slice) soft-supersedes the prior active assistant **attempt**: the old row and its hop captures stay in Postgres (`active=false`); only active messages hydrate into model context and the conversation view. The plane owns truncate/replay via ACP `session/prompt` with `_meta.retryLatest: true`. Mid-thread rewind, edit-and-retry, Attempt navigators, and true thread forks remain deferred; `prompt_message_id` is the foothold for later forks.

**Why:** Day-to-day retest without burning prior turns as input tokens, without destroying inspectable history or inventing client-owned transcripts.

---

## 16. Plane-owned web tool integrations

**Status:** accepted

V1 exposes exactly two stable agent tools — `web_search` and `fetch_page` — backed by first-class catalog `tool_integrations` (kinds: SearXNG, Linkup, get-md, Crawl4AI). A kind may advertise **multiple capabilities** (Linkup: search + hosted fetch); others are single-cap. Plane defaults plus `assistant.settings.toolBindings` resolve at `session/new` and pin into the session. Secrets are write-only on GET/list. Drivers may be native HTTP or remote Streamable HTTP MCP (Linkup); both advertise origin `mcp` so Gate, ACP tool presentation, transcript parts, and hop captures share one path. get-md/Crawl4AI conversion receives plane-fetched bytes (SSRF-safe); Linkup `fetch_page` uses upstream `linkup-fetch` after URL policy checks. No silent provider fallback.

This is the first narrow consumer of [#62](https://github.com/tryy3/agent-fabric/issues/62) (Streamable HTTP MCP initialize/list/call only). Generic MCP marketplace, stdio, OAuth, resources, prompts, and MCP-over-ACP remain deferred.

**Why:** Personal web prototyping needs bounded search-and-read without browser automation or client-owned credentials.

---

## 17. Incremental attempt persistence and cancel (#52 / #55)

**Status:** accepted

Bound turns persist incrementally, not only on successful end-of-turn commit. The plane **Begin**s a user message plus an assistant **attempt** (`status=running`) before the first provider request, **Checkpoint**s complete logical parts (flushed thought, finished tool call, message segment) as they complete, and **Finalize**s the attempt as `completed`, `failed`, or `cancelled`. On provider/stream failure the plane flushes any buffered in-round message text to ACP and catalog, appends an **error** message part (`type=error`) with a short context prefix plus the scrubbed upstream detail, and still returns an RPC error on `session/prompt`. User stop uses ACP `session/cancel`; the prompt returns `StopReasonCancelled` (success) and keeps the prompt plus any partial parts. Abandoned `running` rows become `failed` with `stopReason=interrupted` after a plane restart. Hop captures link on finalize (including cancel/fail). Model context hydrates active users always and active assistants only when `status=completed`; cancelled/failed attempts stay visible in the conversation view. Soft-supersede retry ([§15](#15-latest-prompt-retry-is-soft-supersede-v1)) still restores the prior completed attempt when a draft retry is cancelled or fails. Older “cancel writes nothing” specs are superseded by this decision.

**Why:** Stop, failure UX, reconnect, and reload need a durable partial turn; all-or-nothing CommitTurn erased accepted prompts and streamed parts on cancel, provider error, or plane crash.

---

## 18. Turn failures stay in the transcript (#51)

**Status:** accepted

Inference and other in-turn failures are conversation events, not shell/connection death. The Workbench keeps the user prompt and any partial thought/tool/message output, shows an inline **request failed** activity (amber, like a failed tool) with transparent third-party detail plus helpful context, and leaves the composer usable (`ChatStatus` stays connected). Page-level `ChatStatus.error` / Offline is reserved for failing to establish or load the ACP connection or thread itself—not a single bad provider request. Reloading the thread must reproduce the same failed-turn chrome from catalog `status=failed` and the durable error part.

**Why:** Operators need to see what the provider returned without losing context or restarting the client; wiping the document into a bare error state hid evidence and blocked retry.

---

## 19. Platform, Assistant, and Runtime Context instructions (compose + pin)

**Status:** accepted

Three instruction ownership levels:

1. **Platform instructions** — plane-wide string on `GET/PATCH /v1/settings` (`platformInstructions`). Shared guidance for tools, environment work, and task handling across every assistant.
2. **Assistant instructions** — top-level Assistant field (`instructions`), independent of human-facing `description`. Role, expertise, priorities, and communication style.
3. **Runtime context** — plane-wide string on `GET/PATCH /v1/settings` (`runtimeContext`). Session-specific facts (date, timezone, model, workspace) kept separate from platform methodology.

At `session/new` the plane composes **effective instructions** in order Platform → Assistant → Runtime context, wrapping each non-empty segment in stable snake_case tags (`<platform_instructions>`, `<assistant_instructions>`, `<runtime_context>`). Empty sources are omitted. Before pinning, known instruction variables are substituted across all sources: `{{currentDate}}`, `{{timezone}}`, `{{workspaceRoot}}`, `{{modelId}}`. Substitution uses the bound thread's resolved workspace root and the session's current model so the pin is static for the session (system-prompt cache friendly). Catalog edits apply to the next session only. ACP clients cannot inject or override these instructions.

Adapters map `StreamChatOptions.Instructions` (the pinned effective instructions, i.e. the "system prompt") to the correct wire form. Content and order are identical everywhere; only the field differs. Unset/empty values are omitted. Scrubbed provider-request captures include the outbound representation.

| Adapter | Where effective instructions go |
| --- | --- |
| Anthropic Messages | top-level `system` field |
| OpenAI Responses | top-level `instructions` field |
| Chat Completions | first `messages[]` entry, role `system` (never `developer`) |

Anthropic note: if history ever contains `system`-role messages, the adapter appends them after effective instructions in the same `system` field. The plane creates no such messages today.

Per-thread instruction editing before session start, project `AGENTS.md`, and effective-instructions preview UI are deferred.

**Why:** Shared platform policy must not be duplicated on every Assistant; role-specific behavior must not live only in a global field; and volatile session facts must stay separable from stable methodology so prompts can be cached. Keeping composition, variable substitution, and pinning on the plane preserves the client boundary.

---

## 20. Bounded project file tools (#67)

The agent's environment tools are a small, predictable filesystem toolkit instead of a shell: `read_file`, `write_file`, `list_files`, `search_text`, `apply_patch`, `append_file`, `create_directory`, `move_path`, `delete_path`. All paths resolve inside the project root through the existing jail and path policy, and every call goes through the Gate.

- **Patch format:** `apply_patch` takes one plane-parsed **unified diff**. Context and removed lines must match exactly at the stated line numbers (no fuzz, no offset search). All files are validated in memory before any is written; a mismatch, collision, size or binary violation changes nothing. Create (`--- /dev/null`) and edit are supported; delete and rename are refused (use `delete_path` / `move_path`). A file that is CRLF throughout is patched as LF and written back as CRLF; a file mixing CRLF and LF is refused (`unsupported`, use `write_file`).
- **Limits** are named constants in `sandbox/tools/file/limits.go`, not part of any tool schema: 2 MiB per edited or read file (same as the Workbench editor), 1,000 list entries, and search caps of 1,000 files, 32 MiB scanned, 200 total / 50 per-file matches and 256 KiB returned. Truncation is reported (`truncated`, `truncated_reason`), never silent. Binary files (NUL byte in the first 8 KiB, or invalid UTF-8) are rejected by read/edit tools and skipped by search.
- **Ignored directories:** `list_files` and `search_text` skip a fixed list (`.git`, `node_modules`, `dist`, `build`, `.dart_tool`, ...) unless `include_ignored` is set. The list is fixed, not `.gitignore`-driven, so local and Docker behave identically.
- **Traversal** is built on `FS.ReadDir`/`Stat` only, so the local and exec-backed (Docker) filesystems share one implementation and one contract. `move_path` reuses `fsops.Service.Move` (never overwrites).
- **Delete:** `delete_path` removes a file or an **empty** directory only, never the project root or `.git`, and the Gate **always asks** (Allow once / Reject; no session grant).
- **`.git`** is hard-denied by the Gate and refused again inside the tools for every mutating file tool, including `write_file`.
- **Results** are JSON; failures are `{"error","code"}` with codes `not_found`, `exists`, `too_large`, `binary`, `mismatch`, `not_empty`, `protected`, `invalid_args`, `not_dir`, `is_dir`, `unsupported`, `io_error`.
- **Refresh and commit:** every successful mutating tool marks the turn as having changed files, so the existing once-per-turn project auto-commit covers them, and the client refreshes the Workbench tree and open documents after the turn. The client decides to refresh from the ACP tool kind (`edit`, `move`, `delete`) of completed calls, not from tool titles.
- **Not decided here:** tool inputs and results are persisted with the turn as-is (unscrubbed) like `write_file` today; bounded output is the control. Scrubbing persisted tool parts is a separate follow-up.

**Why:** Whole-file `write_file` rewrites are blunt and unsafe for routine edits, and a general shell would be harder to validate, authorize, display and test. Small typed tools keep each action gateable and renderable.

---

## Explicitly deferred

- ACP v2 as default wire format
- Generic MCP marketplace, stdio MCP, OAuth, resources/prompts, and MCP-over-ACP (see [#62](https://github.com/tryy3/agent-fabric/issues/62))
- Vector memory
- Multi-user auth product
- Naming the product
- OpenCode Free as a third built-in type
- Gemini / Jev OpenCode adapters
- Mid-session ACP sampling / temperature config options
- Deep research orchestration, authenticated browsing, JS interaction, screenshots, recursive crawling
- Per-thread instruction overrides, project instruction files, effective-instructions preview UI
