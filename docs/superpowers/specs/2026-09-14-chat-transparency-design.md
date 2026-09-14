# Chat transparency: thinking, usage, and turn metadata

**Date:** 2026-09-14  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-13-threads-history-design.md](./2026-09-13-threads-history-design.md), [2026-09-12-controlplane-openai-inference-design.md](./2026-09-12-controlplane-openai-inference-design.md), Flutter ACP chat client

## Problem

Transparency is a product requirement: when a prompt is sent, the user should be able to see what went to the model, which model and provider ran, and (later) how tools/MCP were intercepted. Today the plane and Unsloth already produce much of that, but the cockpit only shows concatenated assistant text.

The Dart ACP SDK already models eleven `session/update` kinds, including `agent_thought_chunk` and `usage_update`. The client drops every kind except `agent_message_chunk`. The OpenAI streamer only reads `delta.content`, so `reasoning_content`, `usage`, and llama.cpp/`timings` never reach ACP. Thread history stores `{role, content}` only, so even if the live UI showed thinking or tok/s, a refresh would lose it.

Users who want a quick answer must not be buried in that detail. Extra information defaults to collapsed, togglable containers.

## Goals

- Split Unsloth/OpenAI streams into **thinking** vs **visible answer** on the ACP wire (`agent_thought_chunk` vs `agent_message_chunk`).
- Surface **model**, **provider**, token counts, tok/s, prompt eval, TTFT, elapsed time, stop reason, and stream delta count.
- Persist everything the UI shows on the assistant turn (`parts` + turn metadata) so refresh, reconnect, and control-plane restart restore the same transcript.
- Keep LLM history as visible user/assistant **text only** (no thoughts, no stats).
- Default-collapsed Thinking and Stats expanders; always-on caption `model · provider · tok/s`.
- Settings for Thinking/Stats default: collapsed / expanded / hidden.
- Keep CI hermetic (fake SSE, no live Unsloth).

## Non-goals

- `sent` parts (injected first prompts, `AGENTS.md`, skills). Omit the block while the model payload is unmodified user text; add the type later when injection exists.
- Tools, MCP, plans, permission UI.
- Per-chat visibility override (global settings + per-message expanders only).
- Persisting in-flight chunks on cancel/crash (still drop the uncommitted turn).
- ACP `usage_update` cost (we have no price table).
- Showing thoughts or stats to the **model** on later turns.
- Raw ACP/HTTP inspector.
- Encrypting thoughts at rest.
- Mid-thread **agent** change (still locked). Provider is snapshotted per committed turn.

## Approach

**Chosen:** ACP-shaped turn parts (Approach 1).

- Live: plane parses SSE extras and emits ACP thought / message / usage updates. Flutter renders them as they arrive.
- Durable: after a successful prompt, `CommitTurn` stores visible `content` plus an ordered `parts` JSON list and turn metadata (model, provider, stop reason).
- Reload: `GET /v1/threads/{id}` returns those fields; the same widgets render history and live turns.
- Inference hydrate: `runtime.Store` still copies `role` + `content` only.

**Rejected:**

- Extra scalar columns (`thought`, `usage` JSON, …) — one thought blob cannot represent later tool-call sequences.
- Raw ACP event log — perfect fidelity, heavy to store and query for text + thought + stats.

## Architecture

```text
Flutter
  ACP  session/prompt
       ← agent_thought_chunk
       ← agent_message_chunk
       ← usage_update  (timings in _meta)
  HTTP GET /v1/threads/{id}  → message.content + parts + model/provider
  Settings (local): Thinking/Stats default visibility

Control plane
  OpenAI SSE → thought | content | usage/timings | finish_reason
  Agent.Prompt → ACP updates; on success CommitTurn(parts)
  runtime.Store → user/assistant message text for the next LLM call
```

Thoughts, tok/s, model, and provider are cockpit transcript, not model context. Injected system/first prompts, when they exist later, go to the LLM through the provider request and may appear as a `sent` part; they are not stuffed into `messages.content`.

## Data model

Keep the `messages` row. Add `parts jsonb not null default '[]'`. User rows: `content` = prompt text, `parts` = `[]`, no model/provider/stopReason. Assistant rows: `content` = visible answer; `parts` = what the UI renders besides the bubble text.

`GET /v1/threads/{id}` message object:

```json
{
  "id": "msg_…",
  "role": "assistant",
  "content": "visible answer",
  "position": 2,
  "createdAt": "…",
  "model": "qwen3-32b",
  "providerId": "prov_unsloth",
  "providerName": "Unsloth Studio",
  "stopReason": "end_turn",
  "parts": [
    { "type": "thought", "text": "…" },
    { "type": "message", "text": "visible answer" },
    {
      "type": "usage",
      "promptTokens": 23,
      "completionTokens": 1283,
      "totalTokens": 1306,
      "contextUsed": 1306,
      "contextSize": null,
      "promptMs": 68,
      "predictedMs": 36157,
      "ttftMs": 120,
      "elapsedMs": 36220,
      "promptPerSecond": 338.3,
      "predictedPerSecond": 35.5,
      "deltas": 40
    }
  ]
}
```

Rules:

- `thought` omitted when the model emitted none.
- `message` text always equals `content`.
- A successful turn always writes a `usage` part with plane-measured `ttftMs`, `elapsedMs`, and `deltas`. Token counts and tok/s are added when Unsloth/OpenAI sends `usage` / `timings`; omit unknown keys (do not invent zeros).
- `sent` is reserved (`{ "type": "sent", "blocks": […] }`) and is **not** written in this slice.
- Unknown `type` values are stored and skipped in the UI.
- `model` / `providerId` / `providerName` / `stopReason` are assistant-only, filled from the session pin + stream at commit. The client never sends them.

Goose/sqlc: new migration on `messages`. `InsertMessage` / `ListMessages` include `parts`. `CommitTurn` gains an assistant payload (text + parts + metadata), not two raw strings.

`SessionPin` adds `ProviderName` (catalog already has `providers.name`; pin currently copies id/type/url only).

`GET /v1/agents` (and get-one) adds optional `providerName` when a provider is linked so the live caption can render before commit without a second list fetch.

## ACP mapping

| Source | ACP | Persist |
| --- | --- | --- |
| `delta.reasoning_content` | `agent_thought_chunk` | `parts[]` type `thought` |
| `delta.content` | `agent_message_chunk` | `content` + `parts[]` type `message` |
| `usage` + `timings` + plane clocks | `usage_update` (`used` = total/context tokens, `size` if known, no `cost`; timings, tok/s, TTFT, elapsed, deltas in `_meta`) | `parts[]` type `usage` |
| session pin | not an update (client already knows model; providerName from agent) | assistant `model` / `providerId` / `providerName` |
| `finish_reason` | `PromptResponse.stopReason` | `stopReason` |

`stopReason` mapping: `length` → `max_tokens`; `content_filter` → `refusal`; `stop` / missing on a successful stream → `end_turn`. `cancelled` and transport errors do not commit.

OpenAI request: keep `stream: true`; set `stream_options: { "include_usage": true }` so the final chunk can carry `usage` (and Unsloth `timings`). The last usage chunk often has `choices: []` — that is not an empty assistant.

`ChatStreamer` callback becomes a small event, not `func(string)`:

```text
thought delta | content delta | usage snapshot (usually once at end)
```

One SSE line may carry thought, content, and/or usage; emit the matching ACP updates independently.

## Components

| Piece | Job |
| --- | --- |
| `provider.OpenAI` | Parse `reasoning_content`, `content`, `usage`, `timings`, `finish_reason`; measure TTFT (first thought or content) and elapsed; request `include_usage`. |
| `provider.ChatStreamer` | Event callback above; fakes implement the same. |
| `internal/agent` | Emit thought/message/usage updates; commit parts; hydrate LLM messages from `content` only. |
| `internal/catalog` | `parts` + assistant metadata on messages; `providerName` on agents. |
| `AgentConnection` | Forward thought, message, and usage updates (still ignore other kinds). Permission handler stays cancelled. |
| `ChatController` / `ChatMessage` | One assistant turn with thought, answer, caption fields, usage — not a single string. |
| Chat widgets | Answer, caption, Thinking expander, Stats expander. |
| Settings → Chat tab | Thinking and Stats default: collapsed / expanded / hidden. Stored in client `shared_preferences` (web: localStorage). Not catalog. |

## Chat UI

Transcript stays user bubbles + assistant bubbles. Extra information hangs off the assistant turn.

After a completed turn:

- Assistant answer (always).
- Caption: **model · provider · tok/s** (tok/s omitted if unknown). Always visible in this slice; not gated by Stats hidden.
- **Thinking** expander if a thought exists. Default **collapsed** after the turn completes.
- **Stats** expander (every successful assistant turn). Default **collapsed**. Contents: token counts and tok/s when known, prompt eval, TTFT, elapsed, stop reason, delta count.

While streaming: Thinking **opens** on the first thought chunk, then **collapses when the turn finishes**, unless settings are `expanded` (stay open) or `hidden` (never shown). Caption shows model · provider immediately from the current session/agent; tok/s appears when usage lands.

Settings (global):

- Thinking: collapsed (default) / expanded / hidden
- Stats: collapsed (default) / expanded / hidden

`hidden` means the expander is not shown; the data is still persisted. Per-message expand/collapse is local and not saved; reload follows settings. History uses the same widgets as the live turn. No Sent block.

## Data flow

**Live send:** user prompt → ACP `session/prompt` → SSE parsed → thought/message/usage updates → Flutter accumulates one assistant turn → on success `CommitTurn` → optional `GET` refresh (title / persisted parts). Model request body is existing `content` history + this user prompt.

**Reload:** `GET` thread detail → render parts. `session/new` hydrates inference from `content` only.

**Next turn:** `StreamChat` messages are `[{role, content}, …]` with assistant `content` = visible answer, never thought text.

## Errors and lifecycle

| Case | Behavior |
| --- | --- |
| Cancel, HTTP/SSE failure, crash mid-token | No DB rows; drop uncommitted local bubbles (today). |
| Thinking but no `content` | Empty assistant error; nothing saved. |
| Successful stream, no thinking | No Thinking expander. |
| No Unsloth `usage`/`timings` | Caption is model · provider (no tok/s). Stats still shows TTFT, elapsed, deltas, stop reason. |
| `max_tokens` / `refusal` | **Commit** the turn; show `stopReason` in Stats. |
| Unknown `parts[].type` | Skip in UI; still show `content`. |
| Unknown ACP `session/update` kind | Ignore. |
| CLI / unbound session (`threadId` omitted) | Stream thought/usage over ACP; do not write threads. |

## Testing

**Provider (httptest SSE)**

- Content-only stream → content events only.
- `reasoning_content` then content → thought then content.
- Final chunk `choices: []` with `usage` + `timings` → usage event, not an empty-assistant error.
- Thinking and no content → empty-assistant error.
- `include_usage` present on the outbound JSON.

**Agent**

- Fake streamer thought + text + usage → ACP thought, message, usage updates in that family of order.
- Bound `CommitTurn` stores `content` = visible text, `parts` matching, pin model/provider, mapped `stopReason`.
- Cancel writes zero rows.
- Next `StreamChat` messages omit thought.

**Catalog**

- Round-trip `parts` and assistant metadata on `GET`.
- Row with default empty `parts` still loads as today.

**Flutter**

- Connection maps thought ≠ message ≠ usage.
- Controller builds one assistant turn; widget: answer visible, Thinking collapsed after complete, caption shows model · provider.
- Reload from `ThreadDetail` with parts renders the same.
- Settings defaults affect expanders; caption remains.

## Verification

**Manual**

```text
# Thinking model on Unsloth
flutter run -d chrome
# Send a prompt: Thinking opens while streaming, then collapses; answer remains
# Caption: model · provider · tok/s
# Expand Stats: tokens / timings / stop reason
# Refresh the page: same thought, caption, stats
# Settings → Chat: Thinking = hidden → expander gone, data still in GET JSON
```

**Automated**

- `go test ./...` with Docker for DB tests; no API keys.
- `flutter test` with no live network.

## Success criteria

1. A thinking Unsloth stream shows a thought distinct from the answer over ACP, not concatenated into one bubble.
2. Caption shows the model and provider used for that turn; tok/s when timings exist.
3. A successful turn persists thought, usage, model, and provider; refresh restores them.
4. Failed/cancelled turns persist nothing.
5. The next LLM request does not include thought or usage text.
6. Messages without `parts` still render as plain text.
7. Thinking/Stats default to collapsed; settings can expand or hide; caption stays visible.

## Follow-ups (explicitly later)

- `sent` part when the plane injects first prompts, `AGENTS.md`, or skills
- Tool / MCP / plan / permission cards as additional `parts` types
- Per-chat visibility override
- Raw ACP/HTTP inspector
- Context-window `size` when the provider exposes it
- Cost on `usage_update` if we add pricing
- Persisting expander open/closed per message
