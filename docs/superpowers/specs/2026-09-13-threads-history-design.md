# Durable threads and chat history

**Date:** 2026-09-13  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-12-postgres-catalog-design.md](./2026-09-12-postgres-catalog-design.md), [2026-09-12-dynamic-agents-providers-settings-design.md](./2026-09-12-dynamic-agents-providers-settings-design.md), Flutter ACP chat client

## Problem

Chat is a single live ACP session. Picking an agent calls `session/new` and clears the transcript. History lives only in the in-memory `runtime.Store` and disappears on refresh, reconnect, or control-plane restart.

The first-party Flutter cockpit needs a thread list, a way to start a new conversation, and durable transcripts so switching threads or restarting the app restores the same chats. Catalog work already deferred this; architecture says the plane owns history and the client must not send a canonical transcript.

## Goals

- Persist **threads** and **messages** in Postgres (same goose/sqlc/`DATABASE_URL` path as the catalog).
- Catalog HTTP for list / create / rename / load. The client never POSTs message bodies.
- ACP `session/new` with `_meta.threadId` hydrates a throwaway live session from that thread. Omit `threadId` → today’s in-memory session (CLI and tests).
- Plane appends user + assistant **together** after a successful prompt; cancel/error writes neither.
- Flutter: Chat rail destination shows a **thread pane** (new, title filter, list, overflow rename) beside the existing chat screen.
- New thread title is `Untitled` (`titleSource=auto`). First committed user prompt sets the title to the first **8** whitespace-separated words. A user rename (`titleSource=user`) is never overwritten.
- On Chat open: select the most recently updated thread if any exist; if the list is empty, show an empty pane until **+**. Do not auto-create a thread.
- Keep CI hermetic (testcontainers + fakes; no live LLM).

## Non-goals

- Delete thread (overflow should leave room for it later)
- Default-agent setting (picker stays required before send)
- Server-side search / full-text over messages (client filters titles only)
- Message or thread list pagination
- ACP `loadSession` / persisting ACP session ids
- Mid-thread **agent** change (agent is locked once pinned)
- Auth / multi-user / tenancy
- Encrypting message content at rest
- Client-owned local history (IndexedDB, files)

## Approach

**Chosen:** Catalog threads + plane-written messages (Approach 1).

- Threads are a catalog HTTP resource. Messages are rows written only by the ACP prompt-commit path.
- A live ACP session is an ephemeral handle: new id each `session/new`, discarded on disconnect. Resume is `GET` thread + `session/new` with the same `threadId`.
- Flutter uses HTTP for the index and transcript, ACP for streaming turns.

**Rejected:**

- Persisting `runtime.Session` ids as threads — ACP sessions are throwaway; reconnect still needs a new `session/new`.
- JSONB transcript blob on the thread row — rewrites the whole history every turn; harder to query later.
- Browser-only history — contradicts plane-owned memory and would not show up for a TUI/IDE.

## Architecture

```text
Flutter
  NavigationRail: Chat | Settings
  Chat selected → Thread pane | Chat screen
       │                              │
       │ HTTP                         │ ACP WebSocket
       ▼                              ▼
  GET/POST/PATCH /v1/threads     initialize  (loadSession stays false)
  GET /v1/threads/{id}           session/new  (_meta.agentId + _meta.threadId)
                                 session/prompt  → commit both rows
                                 session/cancel  → write nothing
                                 set_config_option(model) → threads.current_model

Control plane
  Postgres: threads + messages
  runtime.Store: live session hydrated from the thread, not the source of truth
```

| Piece | Job |
| --- | --- |
| `GET/POST/PATCH /v1/threads` | List metadata, create untitled, rename. |
| `GET /v1/threads/{id}` | Thread + messages for the chat pane. |
| ACP `session/new` | Optional `_meta.threadId`: pin agent, restore model, copy messages into `runtime.Store`. |
| ACP `session/prompt` | Stream as today. On success, insert user+assistant and maybe auto-title. |
| CLI / tests | Omit `threadId` → in-memory only; no HTTP thread writes. |

This slice **requires** the Postgres catalog slice (agents table for `threads.agent_id`, same pool/migrations).

## Data model

Same goose/sqlc layout as catalog (`internal/db`). Ids use the existing catalog helper: `th_` + hex, `msg_` + hex.

### `threads`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `text` PK | `th_…` |
| `title` | `text` | Default `Untitled` |
| `title_source` | `text` | `auto` or `user` |
| `agent_id` | `text` nullable FK → `agents(id)` | Set on first `session/new` for this thread; then locked. `ON DELETE RESTRICT` |
| `current_model` | `text` nullable | Last ACP model; restored on reopen |
| `created_at` | `timestamptz` | |
| `updated_at` | `timestamptz` | Bumped on rename, agent pin, model change, committed turn |

### `messages`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `text` PK | `msg_…` |
| `thread_id` | `text` FK → `threads(id)` `ON DELETE CASCADE` | Cascade is for a future delete; this slice has no delete API |
| `role` | `text` | `user` or `assistant` |
| `content` | `text` | Full text of a **committed** turn |
| `position` | `int` | Monotonic per thread; unique `(thread_id, position)` |
| `created_at` | `timestamptz` | |

**Integrity:** `DELETE /v1/agents/{id}` fails with `ErrAgentInUse` (HTTP 409) if any thread references that agent — same pattern as `ErrProviderInUse`.

## HTTP API

JSON camelCase, error body `{ "error": "…" }` like the rest of the catalog. `POST` create returns **201**.

List items are metadata only (no message bodies). `messageCount` lets the sidebar show empty vs not.

```text
GET    /v1/threads           → 200  [ThreadListItem, …]  ordered by updatedAt desc
POST   /v1/threads           → 201  Thread  (body {} or omitted)
GET    /v1/threads/{id}      → 200  ThreadDetail
PATCH  /v1/threads/{id}      → 200  Thread  body { "title": "…" }
```

No `POST /v1/threads/{id}/messages`. No pagination this slice (return the full list).

**ThreadListItem:** `id`, `title`, `titleSource`, `agentId` (nullable), `currentModel` (nullable), `messageCount`, `createdAt`, `updatedAt`.

**Thread:** list fields without `messageCount`.

**ThreadDetail:** Thread plus `messages: [{ id, role, content, position, createdAt }]` in `position` order.

**PATCH title:** trim whitespace; reject empty (400). Sets `titleSource=user` and bumps `updatedAt`. Does not change `agentId` or messages.

**404** if the thread id does not exist. **409** on agent delete while threads exist.

## ACP binding

`initialize` keeps `loadSession: false`. Resume is our HTTP + `session/new`, not ACP session load.

`_meta.threadId` is optional (string). `_meta.agentId` stays required for `session/new` (unchanged for CLI).

When `threadId` is present:

1. Load the thread; missing → ACP error (not found).
2. If `agent_id` is null, set it from `_meta.agentId` and bump `updatedAt`.
3. If `agent_id` is already set, `_meta.agentId` must match; otherwise error (agent locked).
4. Create a new runtime session id (not equal to `threadId`). Copy DB messages into `runtime.Session.Messages` in order.
5. If `current_model` is set and still in the pin’s model list, use it; else the agent default (and persist that onto the thread if `current_model` was empty).
6. Return ACP `sessionId` + `configOptions` as today.

When `threadId` is absent: current behavior; prompt commit does not touch `threads` / `messages`.

`session/prompt` with a bound thread:

- Stream into the live session as today. Build the model request as `existing thread messages + this user prompt` without appending to `runtime.Store` or Postgres until commit.
- **Commit both roles in one DB transaction after the turn succeeds:** next two `position`s, user content = prompt text, assistant content = full assembled text. Then append the same two messages onto the live `runtime.Session`.
- Then, if `title_source=auto`, set `title` to the first 8 whitespace-separated words of that user prompt (`strings.Fields` / equivalent). If there are fewer than 8 words, use all of them. Do not append an ellipsis. Leave `title_source=auto`.
- If `title_source=user`, do not change `title`.
- Bump `threads.updated_at`.
- Cancel or prompt error: **write no new message rows**, do not auto-title, and do not append to `runtime.Store`. A later retry or `GET` must not see a dangling user turn or a partial assistant.

`session/set_config_option` for model: existing pin update **and** `threads.current_model` + `updated_at` when the session is bound to a thread.

`session/cancel`: existing abort; no message rows.

## Flutter UI

Thread pane is a sibling of the chat screen, **only when Chat is selected**. Settings stays rail + settings (IndexedStack still keeps Chat alive so ACP does not reconnect).

```text
[ Chat ]     Threads              Agent Fabric
[Settings]   [+]                   [Agent ▾] [Model ▾]
             [filter        ]      transcript…
             How do I pin…  ⋮      composer
             Untitled
             Summarize logs
```

- **+** → `POST /v1/threads`, select it, empty transcript, no ACP session, composer disabled until an agent is picked. Multiple empty untitled threads are allowed.
- Filter: case-insensitive substring on **title** over the already-loaded list. No extra API. Empty filter results leave the current thread selected.
- List row: title (ellipsis), optional quiet subtitle (`empty` when `messageCount==0`). Selected row highlighted.
- Overflow **⋮**: visible on pointer hover **and** on the selected row (works without hover). Menu this slice: **Rename** only. Rename opens a dialog; save → `PATCH` title.
- Agent picker: enabled when the selected thread has no `agentId`; disabled (locked) once pinned. Model picker: enabled when the ACP session for that thread is ready; changing model uses ACP as today.
- No thread selected: short empty state (“Create a thread to start chatting”); composer and pickers disabled.
- Switching threads while a prompt is in flight: call ACP `session/cancel` first; if cancel fails, stay on the current thread and show the status error. On success, drop uncommitted local bubbles, then load the other thread.
- After a successful prompt: re-`GET` the current thread (or list) so auto-title and `updatedAt` match the plane. Optimistic local title using the same 8-word rule is allowed until that GET returns.
- On prompt failure or cancel: drop the uncommitted local user+assistant pair so the pane matches durable history.

Client session API grows `startSession(agentId, {threadId})` and `cancel()`; `threadId` is passed as `_meta.threadId`.

Follow existing `CatalogClient` + `ChatController` patterns (no new state-management framework).

## Data flow

**Open app:** connect ACP (no session) → `GET /v1/threads` → if non-empty, select newest → `GET /v1/threads/{id}` → if `agentId` set, `session/new` with that agent + thread.

**New thread:** `POST` → select → pick agent → `session/new` pins `agent_id` → composer on → first successful prompt commits messages and auto-title.

**Reopen a thread:** `GET` detail for the pane; `session/new` hydrates inference history. The UI transcript comes from HTTP, not by replaying ACP events.

**Rename:** overflow → dialog → `PATCH` → update the row in the in-memory list.

## Errors and lifecycle

| Case | Behavior |
| --- | --- |
| Threads HTTP failure | Status line error; keep current list/transcript; do not fake create/rename success |
| Thread missing on GET or `session/new` | Drop selection, empty state, refresh list |
| `session/new` fails | Thread stays selected with HTTP messages; composer off; status error |
| Prompt fails / cancel | No new DB rows; drop uncommitted local bubbles |
| Filter matches nothing | Empty list UI; keep current selection |
| Rename empty/whitespace | Dialog validation; no PATCH |
| Switch during send | Cancel first; on cancel failure, do not switch |
| Agent delete with threads | 409 `ErrAgentInUse` |
| ACP disconnect | Existing disconnected status; selected thread unchanged; after reconnect, `session/new` again if an agent is pinned |
| CLI omits `threadId` | In-memory session; no thread writes |

## Components & layout

```text
controlplane/
  internal/db/            # goose: threads + messages; sqlc queries
  internal/catalog/       # HTTP /v1/threads; ErrAgentInUse on agent delete
  internal/agent/         # threadId hydrate; commit turn; persist model
  internal/runtime/       # Create session preloaded with messages
client/lib/
  catalog/                # Thread models + CatalogClient methods
  chat/                   # Thread pane; ChatController list/selection/switch
  acp/                    # startSession threadId; cancel()
  app_shell.dart          # Thread pane only when Chat is selected
```

ACP agent depends on a small thread-store interface (get/pin/commit/set model), not on HTTP handlers.

## Testing

**Control plane (testcontainers Postgres)**

- Create untitled; list `updatedAt` desc; rename sets `titleSource=user`; empty title rejected.
- Agent delete blocked while threads reference it (`ErrAgentInUse`).
- `session/new` with `threadId` hydrates messages into the runtime session; without `threadId` writes nothing to threads.
- Prompt commit inserts user+assistant in one transaction; cancel/error inserts nothing; first prompt auto-titles; renamed thread keeps title.
- `set_config_option` persists `current_model`; reopen restores it.
- Second `session/new` on the same thread with a different agent fails.

**Flutter**

- Chat shows the thread pane; Settings does not.
- +, title filter, overflow rename dialog, selected row.
- Controller: most-recent on connect; empty list → empty state; create/select/switch; send disabled until agent is set; switch cancels in-flight send and drops uncommitted bubbles.
- Fakes for catalog + ACP (same pattern as current tests).

## Verification

**Manual**

```text
# Postgres catalog slice already running
flutter run -d chrome
# Chat: empty state → + → pick agent → send → sidebar title updates from the prompt
# New thread, switch back, refresh the page: both threads and transcripts remain
# ⋮ → Rename; next prompt does not overwrite that title
# Settings: thread pane gone; return to Chat without reconnecting ACP
```

**Automated**

- `go test ./...` with Docker for DB tests; no API keys.
- `flutter test` with no live network.

## Success criteria

1. `POST /v1/threads` creates `Untitled`; `PATCH` renames; `GET` list is newest first.
2. A successful ACP prompt on a bound thread persists both messages and auto-titles when `titleSource=auto`.
3. Cancel/error persist nothing; a renamed title survives the next prompt.
4. Refreshing Flutter restores the thread list and the selected transcript from HTTP, then resumes ACP via a new `session/new`.
5. CLI/`session/new` without `threadId` is unchanged.
6. Agent delete while threads exist returns 409.
7. Thread pane is only on Chat; filter, +, and rename work without a server search API.

## Follow-ups (explicitly later)

- Delete (and other overflow actions)
- Default agent on new thread
- Server search (`GET /v1/threads?q=`) including message text
- Pagination of threads and long transcripts
- Mid-thread agent change / handoff
- ACP `loadSession` if we ever want protocol-native resume
- Auth, encryption at rest
