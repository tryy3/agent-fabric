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

Before sandbox tools run, a pluggable **Gate** returns `allow`, `ask`, or `deny`. Deterministic rules decide first; optional scorer models join as tiers of a cascade without changing the agent loop (risk scores, permission modes and the cascade: decision 22). `ask` uses ACP `session/request_permission` (Allow once / Allow for this session / Reject). `deny` fails the tool with no prompt.

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

## 21. Sandboxed run_command (#68)

**Status:** accepted

The agent can run one non-interactive command in the project's Docker environment through the environment-origin tool **run_command**. It takes an **argv array** (never an implicit shell string), an optional project-relative cwd, optional stdin and a bounded timeout (default 120 s, max 15 min), and returns exit code, stdout, stderr, duration and truncation flags. A non-zero exit code is a normal result, not a tool failure. Output is capped at 64 KiB per stream while the command runs (the executors drain but stop storing); timeout and Stop (session/cancel) end the command. Commands never use ACP client terminal methods.

The Gate classifies every call deterministically from argv, first match wins:

1. **Deny:** any environment that is not Docker (local/host execution stays off in v1), cwd escaping the project root, privilege or host-control programs (sudo, mount, docker, ...), destructive commands aimed at sensitive paths or .git, a recursive or forced rm/chmod/chown/shred of the filesystem root, the home directory or the project's parent, and a netcat that would open a remote shell.
2. **Ask, no session grant:** destructive commands (rm, mv, chmod, find -delete/-exec, destructive git), scored higher the further they reach (one file 6, recursive or forced 7, the whole project 8, process 1 9); network programs (curl, wget, ssh, scp, rsync, nc, ...; 6, or 9 when an argument is a secret, system or home path); and unclassifiable commands (shell -c strings, inline interpreter code, wrappers like env/xargs; 6). A shell string or wrapped command is also classified by what it would run, so `sh -c "rm -rf /"` is denied and a download piped into a shell scores 9. A forbidden program name reached by an unusual path (`/tmp/sudo`) asks at 7.
3. **Allow:** a small read-only allowlist (ls, cat, grep, git status/diff/log, --version, ...) whose path arguments stay in the project; an outside path downgrades to ask. `~` and `$HOME` count as outside the project although argv execution does not expand them. Reading a secret file (.env, private keys) scores 3.
4. **Ask with a session grant:** everything else, including build, test and install tooling. A path argument outside the project asks without a grant instead. "Allow for this session" stores a **command grant** keyed on the command prefix (npm test, npm run build, go test, python3 script.py); it never widens beyond that prefix and never overrides tiers 1-2.

Approving a command does not elevate the sandbox (no path grant). The ACP permission request carries the command, cwd and grant key in rawInput so clients show exactly what will run. A completed run_command marks the turn as having changed files so the existing auto-commit and Workbench refresh cover generated output; the client refreshes on ACP kind execute.

- **Not decided here:** (an LLM or scoring evaluator and per-assistant permission rules landed in decision 22) persisted grants across plane restart, scrubbing of persisted command input/output (same open item as decision 20), a Workbench command console.
- **Known gap:** cancelling a docker exec ends the client; whether the in-container process dies depends on the runtime. The Docker integration test TestDockerRunCommandCancelKillsProcess asserts it.

**Why:** Builds and tests are release-blocking for v1, but a shell is the least predictable tool. Argv-only calls are classifiable; tiers keep trivial commands friction-free while irreversible or opaque ones always reach the user.

---

## 22. Risk scores and permission modes (#58)

**Status:** accepted

Every Gate evaluation carries a **risk score** from 1 to 10 next to its allow/ask/deny verdict, and a user-facing **permission mode** decides what each score means. The deterministic rules score every decision (reads 1, in-project edits 2, build/test tooling 4, delete 5, destructive or opaque commands 6-7, path escapes needing elevation 7, host-control and protected paths 10). Structural refusals (bad arguments, a non-Docker environment, a working directory outside the project) stay unscored and are never relaxed.

The gate is a **cascade** of up to three tiers, built per session from the pinned settings:

1. **Rules**, always. A hard deny, a permission rule's verdict, a score of 9-10, a score of 2 or less (`skipAtOrBelow`), or a **settled** rule tier is final. Tiers the rules judge reliably are settled and never reach a scorer: read-only commands, network commands, exfiltration, forbidden programs, paths outside the project, deceptive file names and in-project reads and edits. Scorers are consulted where what the user asked for, or what hidden code does, decides the answer: build/test and other commands, destructive commands, wrappers and inline code, deletes, secret and startup files, and files that run later (CI workflows, build files, scripts). With no scorer configured this is the whole gate, and it works offline.
2. **Fast tier**, optional: a `SystemOneScorer` (a System One decision model through `POST /v1/systemone`, which returns a typed answer and a confidence instead of text). It can raise the rules' score but not lower it. **Jev is the recommended model**; Laya and other models served through the System One API are supported but scored poorly in the gate benchmark.
3. **Deep tier**, optional: an `LLMScorer` (any chat model). It is asked when there is no fast tier, or the fast tier failed, reported a confidence below `minConfidence` (0.6), or scored two or more below the rules. It sees the user's request and the earlier scores. It may raise the score freely but lower it by at most `maxLower` points (default 2) below the highest score the rules or a confident fast tier gave, so a 7 the model calls a 2 becomes a 5 and is still asked about. If it fails, the highest score stands.

Scorers answer in the five bands (scored 1, 3, 5, 7, 9) plus whether the user requested the call, or with a 1-10 score; the benchmark compares both. Live defaults: the deep tier uses bands, the fast tier the 1-10 score (`deep.style`, `fast.strategy`). Scoring needs no reasoning pass, so the deep tier sends thinking off where the provider has the switch (Unsloth Studio; `deep.enableThinking`, `deep.reasoningEffort` and `deep.maxTokens` override), and `deep.structuredOutput` constrains the reply to the answer's JSON schema (`response_format`) on providers that support it. Only rules give a 10. Each scorer exchange is persisted as a scrubbed hop capture (`meta.hop = gate_scorer`).

**Permission rules** let users extend the built-in rules: `settings.permissions.rules` on the assistant, and plane-wide in `permissions.rules` of `PATCH /v1/settings` (plane rules and the assistant's both apply). A rule names a tool (or `*`), a pattern and an action (`allow`, `ask`, `deny`, or `score`, which needs a `risk` of 1-10). Commands match token by token (`git push *`); file tools match a glob over the project-relative path (`**/.env*`, where `*` stays inside one path segment). The most restrictive matching rule wins: `deny` refuses, `ask` always asks, `allow` always runs, **in every permission mode**, and no scorer rescores it. Settings write every rule as a `score` rule: the call gets its `risk`, and the permission mode turns the score into run, ask or cancel like a built-in tier. A rule is settled (no scorer rescores it) unless it sets `consult: true` ("Ask the scorers"), exactly like a built-in tier's `consult`: then the fast and deep tiers see the call with the rule's score as the rules' score. The older actions stay valid in the API and show in Settings as the score they imply (allow 1, deny 10, ask the built-in score). It loses to a matching deny or ask rule, and of two score rules the higher wins. Each tier in `GET /v1/permissions/builtins` also lists the `tools` it applies to (absent: any tool), and Settings shows them. A rule never relaxes a built-in deny. The **built-in rule tiers** are data: `GET /v1/permissions/builtins` lists each tier with its description, action, base score, whether scorers are consulted and, for program-list tiers (forbidden, destructive, network, wrappers, read-only), its programs. `permissions.builtins` overrides a tier by ID: `risk` (base score), `consult`, and `add` / `remove` for program names, plane-wide or per assistant (the assistant's override wins per tier). The `protected` tier (system paths, `.git`, wipes of the system or home directory, remote shells, non-Docker environments, working directories outside the project) is locked. Settings shows the tiers and edits these overrides. Scorers are configured next to the rules in `permissions.scorers` (`fast` and `deep`: an inference connection id and a model, so keys stay in the catalog); the assistant's block replaces the plane-wide one.

**Sandbox network:** a container resource has a `network` mode, `none` (default) or `bridge`. With `none` the container is started with `--network none`, so commands cannot reach other hosts and the agent reaches the web only through the plane's `web_search` and `fetch_page` tools, which the user binds or disables per assistant. Package installs and other downloads then need `bridge`. A running named container with the other mode is refused with a message to remove it, like a changed image or mount list.

A session is **tainted** once it has read web content (`web_search`, `fetch_page`), which may carry instructions planted for the agent. From then on the policy asks from 5 in every mode except `full`, and the deep tier may not lower a score.

Scores group into five **bands**: safe (1-2), low (3-4), elevated (5-6), high (7-8), cancel (9-10). A **permission policy** is two thresholds per mode, ask from and cancel from:

| Mode | Ask from | Cancel from |
|---|---|---|
| `ask` (Ask for approval, default) | 3 | 9 |
| `auto_approve` (Approve for me) | 5 | 9 |
| `auto` (Run automatically) | 7 | 9 |
| `full` (Full access) | never | 10 |

Below the ask threshold a call runs, at or above it the user is asked, at or above the cancel threshold it is aborted and the model sees `denied: ...`. In `full`, a call the sandbox would still block (a path outside the policy) is elevated for that call without a prompt. The mode is `settings.permissions.mode` on the assistant (catalog PATCH); mode, rules and scorers are pinned at `session/new`. Unset mode means `ask`. Permission requests carry `risk`, `band`, `rationale` and every tier's score in rawInput. Thresholds are constants for now (`gate.DefaultPolicies`); user-editable and per-environment policies, a Flutter mode selector, persisted grants and a `full` danger confirmation are not decided here.

**Transparency:** every gated tool call reports what the gate decided in the ACP tool-call update `_meta.gate` (risk, band, mode, outcome such as allowed / approved / rejected / cancelled, verdict, rule, reason, whether a permission rule decided, whether the session is tainted, and each tier's score, confidence and rationale in order, so a score the deep tier lowered stays visible). It is stored on the persisted `tool_call` part (`gate`) so history shows it, and the chat shows a risk badge on the tool call with the details when expanded. It is never part of the tool result the model receives.

The gate benchmark (`cmd/gatebench`, [gate-benchmark.md](gate-benchmark.md)) scores setups against a labelled dataset of ideal scores. Rules cannot see user intent, so cases record where the rules are known to differ from the ideal.

**Why:** A verdict alone cannot express "ask in Ask mode, run in Run automatically". A shared score lets deterministic rules, LLM scorers and future classifiers be combined and compared, and lets users tune autonomy without touching the rules. Benchmarking showed that most calls are settled correctly by rules, that a model is worth its latency only on the ambiguous middle, and that its distinctive contribution is judging whether the user asked for a call; other harnesses (Zed, OpenCode, Cursor, Codex, Claude Code) put deterministic rules and containment first for the same reason. A bounded lowering keeps a model that is talked into "safe" by untrusted arguments from waving a dangerous call through.

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
