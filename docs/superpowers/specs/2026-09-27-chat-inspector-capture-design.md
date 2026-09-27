# Chat Inspector & hop capture

**Date:** 2026-09-27  
**Status:** approved for implementation  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md), root [AGENTS.md](../../../AGENTS.md) (Inter-service capture)  
**Builds on:** [2026-09-18-thread-view-modes-markdown-design.md](./2026-09-18-thread-view-modes-markdown-design.md), [2026-09-14-chat-transparency-design.md](./2026-09-14-chat-transparency-design.md), [2026-09-16-sandbox-tool-calling-design.md](./2026-09-16-sandbox-tool-calling-design.md)  
**Fulfills:** Deferred `rawRequests` / Raw inspector from thread view modes.

## Problem

Thinking, tool I/O, and usage parts show harness activity, but not the exact provider HTTP payload the LLM received. That payload lives only on the control plane at `StreamChat`. Debugging bad turns needs durable, scrubbed hop captures and a cockpit Inspector that does not dump JSON into chat bubbles.

## Goals

- Persist inter-service hop captures on the plane (first hop: provider LLM HTTP per tool-loop round).
- Scrub before persist: Go header/cookie redaction + Nano Collective `prompt-scrub` for bodies (one-way at rest).
- Expose captures via catalog HTTP for lazy load.
- Add built-in **Raw** view mode (`rawRequests: true`) that unlocks Chat | Inspector | Split chrome.
- Minimal Inspector: hop list + Context / Raw tabs for the selected assistant message.

## Non-goals

- Live LLM scrub+rehydrate on the provider path (same scrubber interface later).
- ACP / sandbox / MCP hop capture UI (schema accepts `hop_kind`; only `llm` shipped).
- Embedding captures in `messages.parts` or ACP `session/update`.
- User-editable custom modes.
- Capture retention UI / TTL policies beyond cancel cleanup.
- Persisting every SSE chunk.

## Decisions

| Topic | Choice |
|-------|--------|
| Scope | Vertical slice: capture + scrub + catalog + Raw mode + Inspector shell |
| Storage | Separate `hop_captures` table |
| Scrub | Go headers + `prompt-scrub` CLI for bodies; fail-closed on scrub error |
| Capture retention | One-way (discard prompt-scrub session maps) |
| Delivery | `GET /v1/threads/{id}/messages/{msgId}/captures` |
| Inspector gate | `ViewMode.rawRequests` (Raw preset) |
| Layout | Inside chat: Chat \| Inspector \| Split (EditorPreviewPane pattern) |

## Approach

**Chosen:** Plane-owned capture store + catalog lazy-load + Raw-gated Inspector.

**Rejected:**

- Stuffing captures into `parts[]` — bloats every thread GET.
- Client DevTools as source of truth — violates client boundary / AGENTS.md.
- MITM-only proxy without plane persistence — not durable across reloads.

## Architecture

```text
Flutter (Raw mode)
  Chat | Inspector | Split
  GET /v1/threads/{id}/messages/{msgId}/captures

Control plane
  Agent.Prompt → StreamChat rounds
  Provider HTTP boundary → CaptureSink (raw hop)
  scrub.Pipeline → headers Go + body prompt-scrub
  hop_captures insert; CommitTurn links message_id
```

## Data model

```sql
CREATE TABLE hop_captures (
  id text PRIMARY KEY,
  thread_id text NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  message_id text NULL REFERENCES messages(id) ON DELETE CASCADE,
  session_id text NULL,
  round_index int NOT NULL,
  hop_kind text NOT NULL,
  direction text NOT NULL,
  method text NULL,
  url text NULL,
  status_code int NULL,
  headers_json jsonb NOT NULL,
  body_text text NOT NULL,
  meta_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
```

`direction` for v1: `exchange` (one row per round with request+response folded into body/meta) or separate request/response rows — implementation uses one `exchange` row with request body primary in `body_text` and response assembled JSON in `meta_json.response_body`.

## Scrub pipeline

```go
type HeaderRedactor interface { Redact(http.Header) http.Header }
type ContentScrubber interface { Scrub(ctx context.Context, content string) (string, error) }
```

- Headers: `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, bearer/`sk-` shaped values → `[REDACTED]`.
- Bodies: `prompt-scrub scrub -q` via `PROMPT_SCRUB_BIN` (default `prompt-scrub`); discard session maps.
- Scrub failure: store placeholder text, never raw secrets.
- Unit tests: fake scrubber; no Node required.

## Catalog API

- `GET /v1/threads/{threadId}/messages/{messageId}/captures` — ordered by `round_index`, `created_at`.
- PATCH thread `viewModeId` allowlist: `pretty | detailed | raw`.

## Client

| Mode | markdown | thinking | tools | rawRequests | Inspector |
|------|----------|----------|-------|-------------|-----------|
| pretty | yes | collapsed | collapsed | no | hidden |
| detailed | no | collapsed | collapsed | no | hidden |
| raw | no | expanded | expanded | yes | Chat\|Inspector\|Split |

Inspector tabs: **Context** (scrubbed request body), **Raw** (headers + body + response meta).

## Testing

- Plane: scrub header redaction; fake body scrubber; capture insert + link on CommitTurn; catalog GET; view mode allowlist includes `raw`.
- Client: resolve Raw; Inspector chrome only when `rawRequests`; fetch/render captures widget test with fake catalog.

## Future

- ACP / sandbox hop kinds; live scrub+rehydrate; custom modes; `get-md`; capture TTL; in-flight session capture listing.
