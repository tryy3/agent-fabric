# Dynamic agents: providers catalog + agent settings

**Date:** 2026-09-12  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-12-controlplane-openai-inference-design.md](./2026-09-12-controlplane-openai-inference-design.md), Flutter ACP chat client

## Problem

The POC chats through a single hardcoded agent definition and process-global `OPENAI_*` env config. There is no way to register multiple OpenAI-compatible endpoints, inspect their `/v1/models` lists, or create agent definitions that bind a provider + default model. Settings and runtime stay collapsed into env vars.

We need the base catalog layer and Flutter settings surfaces so providers and agents can be configured independently, with chat selecting an agent and switching models mid-session via ACP.

## Goals

- **Catalog HTTP API** for providers and agents, persisted as local JSON (Go stdlib only).
- **Providers:** CRUD OpenAI-compatible connections (`baseUrl`, `apiKey`); explicit **Refresh models** that caches `GET {baseUrl}/models`.
- **Agents:** CRUD definitions that link `providerId` + `defaultModel`; UI placeholders for tools / MCP / sandbox / memory (not executed yet).
- **Provider type registry:** `openai_compatible` implemented; design leaves room for future adapter types.
- **ACP:** `session/new` takes `agentId` (via `_meta.agentId`), pins a definition snapshot, returns `configOptions` with `category: "model"` from the provider’s **cached** models; implement `session/set_config_option` for model.
- **Chat:** agent picker starts a new ACP session; model selector uses ACP config options.
- **Flutter shell:** sidebar **Chat** · **Settings**; Settings tabs **Providers** | **Agents**.
- Drop the startup requirement on `OPENAI_*` (empty catalog is valid).
- Keep CI offline with fakes; no new Go dependencies for JSON storage.

## Non-goals

- Catalog auth / multi-user
- Encrypting API keys at rest
- Executing MCP, tools, sandbox, or memory (placeholders only)
- Mid-session **agent** switching or a separate “thread” entity beyond ACP sessions
- Persisted multi-chat history browser
- Non-OpenAI provider adapters beyond the type string + registry stub
- Env-var bootstrap / seed of providers
- ACP Registry marketplace integration

## Approach

**Chosen:** Catalog resources + ACP session binding (Approach 1).

- HTTP catalog owns CRUD and model refresh.
- ACP owns runtime chat; inference resolves from the pinned agent’s provider.
- Mid-session model changes use [ACP session config options](https://agentclientprotocol.com/protocol/v1/session-config-options) (`category: "model"`), not a second catalog round-trip per turn.
- New chat/thread = new ACP `session/new` against a chosen agent. Agent stays fixed for that session; model may change.

**Rejected:**

- Settings-only file edits from the client (secrets/policy leave the plane).
- Single mixed “runtime profile” blob (harder to evolve providers vs agents).
- Binding agent only at WebSocket connect time (switching agent would force reconnect; `session/new` is enough).

## Architecture

```text
Flutter Settings ──HTTP──► Catalog API ──JSON──► providers.json / agents.json
Flutter Chat     ──WS────► ACP /acp
                              │
                              ├─ session/new (agentId) → pin definition + model configOptions
                              ├─ set_config_option(model)
                              └─ prompt → provider adapter (openai_compatible → Chat Completions SSE)
```

| Layer | Role |
| --- | --- |
| **Providers** | Connection configs + cached model list. Not ACP peers. |
| **Agents** | Catalog definitions (logical ACP agents). Reference provider + default model. |
| **ACP session** | One chat thread with one pinned agent; model selectable via configOptions. |
| **Inference** | Adapter registry keyed by provider `type`. |

## Data model & persistence

**Location:** `{dataDir}/providers.json` and `{dataDir}/agents.json` (default `./data`, overridable by flag). Atomic write (temp + rename). Stdlib `encoding/json` only.

**API keys:** plaintext in JSON for this local-dev slice; encryption called out as follow-up.

### Provider

| Field | Notes |
| --- | --- |
| `id`, `name` | Stable id |
| `type` | `openai_compatible` now; string for future adapters |
| `baseUrl`, `apiKey` | Used for Chat Completions and `/models` |
| `models[]` | Cached: at least `id`, display `name`; optional extras from upstream |
| `modelsUpdatedAt` | nullable |
| `createdAt`, `updatedAt` | |

### Agent

| Field | Notes |
| --- | --- |
| `id`, `name`, `description?` | |
| `version` | Bump on each update (session pin) |
| `providerId`, `defaultModel` | Required. Saving requires `defaultModel` ∈ that provider’s cached models (refresh first if empty). |
| Placeholders | `tools`, `mcpServers`, `sandbox`, `memory` — empty / omitted; Flutter shows disabled sections |

## Catalog HTTP API

No auth this slice.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/v1/providers` | List |
| `POST` | `/v1/providers` | Create |
| `GET` | `/v1/providers/{id}` | Get |
| `PATCH` | `/v1/providers/{id}` | Update |
| `DELETE` | `/v1/providers/{id}` | Delete; `409` if any agent still references it |
| `POST` | `/v1/providers/{id}/models/refresh` | `GET {baseUrl}/models`, replace cache |
| `GET` | `/v1/agents` | List |
| `POST` | `/v1/agents` | Create |
| `GET` | `/v1/agents/{id}` | Get |
| `PATCH` | `/v1/agents/{id}` | Update (bump `version`) |
| `DELETE` | `/v1/agents/{id}` | Delete |

Same process serves catalog HTTP and ACP WebSocket (e.g. `:8080`).

## ACP runtime

### `session/new`

1. Read `agentId` from request `_meta.agentId` (documented extension).
2. Load agent + provider from catalog; fail if missing.
3. Pin snapshot on the session: definition fields, provider credentials/type needed for inference, **model option list from cache at pin time**, `currentModel = defaultModel`.
4. Return `configOptions`: one `select` with `id: "model"`, `category: "model"`, `currentValue` = default, `options` from the pinned cached list.
5. If cache is empty / default unusable → JSON-RPC error directing the user to refresh models in Settings.

Remove hardcoded `echo` from the live path. Tests use fakes / scripted streamers.

### `session/set_config_option`

- Implement for `configId: "model"` only.
- Value must be in the **pinned** model list (provider refresh does not mutate in-flight sessions).
- Respond with full `configOptions` state per ACP.

### `session/prompt`

- Resolve a `ChatStreamer` from the provider type registry for the pinned provider.
- Pass the session’s **current** model into the stream call (extend today’s process-global model so the adapter is per-request / per-session).
- No process-global `OPENAI_*` config for the live path.

### Agent vs model switching

| User action | Mechanism |
| --- | --- |
| New chat / switch agent | New ACP `session/new` with that `agentId` (same `/acp` connection OK). UI starts a fresh transcript for the new session. |
| Switch model mid-chat | `session/set_config_option` on `model`; transcript continues. |

## Flutter UI

- **Shell:** left sidebar — **Chat**, **Settings** (room for future tools).
- **Settings:** tab bar — **Providers** | **Agents**.
- **Providers tab:** list/create/edit; fields name, type (fixed openai_compatible for now), base URL, API key; **Refresh models**; show cached list + last updated.
- **Agents tab:** list/create/edit; name, description, provider picker, default model picker (from that provider’s cache); placeholder panels for tools/MCP/sandbox/memory.
- **Chat:** agent dropdown (catalog `GET /v1/agents`) → new session with `_meta.agentId`; model dropdown bound to ACP `configOptions`.
- Catalog HTTP base and ACP WS URL: client constants for now (e.g. `http://localhost:8080`, `ws://localhost:8080/acp`), editable later.

## Error handling

| Case | Behavior |
| --- | --- |
| Unknown provider/agent id | `404` |
| Validation (missing fields, bad URL, defaultModel not in cache, unknown providerId) | `400` with clear message |
| Delete provider still referenced by agents | `409` |
| Models refresh upstream failure | Error response + log upstream body; keep previous cache |
| `session/new` bad/missing agentId or empty models | ACP JSON-RPC error |
| `set_config_option` unknown model | Error; keep previous model |
| Prompt/stream failure | Existing log + ACP error path |

## Testing

**Go (offline):**

- Catalog store CRUD + JSON round-trip / atomic write
- Models refresh against fake HTTP upstream
- Agent validation against provider cache
- Provider delete conflict when referenced
- ACP: `session/new` pins definition and returns model `configOptions`
- ACP: `set_config_option` changes model recorded by fake streamer on next prompt

**Flutter:**

- Catalog client parsing with mocked HTTP
- Settings list/create flows mocked
- Agent picker issues `session/new` with `_meta.agentId`
- Model UI reflects `configOptions` (mocked ACP)

## Follow-ups

- SQLite (or similar) replacing JSON files
- Encrypt secrets at rest; catalog auth
- Wire MCP / tools / sandbox / memory from agent placeholders
- Additional provider adapters behind the type registry
- Optional agent allowlist for ACP model options (subset of provider cache)
- Multi-chat history persistence; mid-session agent handoff across a plane-owned thread
- Editable catalog/ACP URLs in the client
