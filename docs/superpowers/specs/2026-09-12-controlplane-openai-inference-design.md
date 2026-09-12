# Control plane OpenAI Chat Completions inference

**Date:** 2026-09-12  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-11-controlplane-acp-echo-design.md](./2026-09-11-controlplane-acp-echo-design.md), Flutter ACP chat client

## Problem

The ACP path works end-to-end (Flutter / CLI → WebSocket `/acp` → agent → `session/update`), but the live provider only echoes user text. There is no server-side transcript and no call to a real model. We need a thin vertical slice that proves **prompt → OpenAI-compatible Chat Completions → streamed reply** against a local Unsloth Studio endpoint.

## Goals

- Replace the live echo path with OpenAI Chat Completions streaming (`stream: true`).
- Pass full in-memory session history as Chat Completions `messages` on each turn.
- Configure inference via env only: `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL` (fail fast at startup if any missing).
- Stream token deltas as ACP `session/update` (`agent_message_chunk`) so the existing Flutter client shows progressive output.
- Support `session/cancel` by cancelling the in-flight HTTP request; do not commit a partial assistant turn to history.
- Keep CI offline: fake OpenAI HTTP + in-process ACP tests; document Unsloth smoke in README.

## Non-goals

- Catalog HTTP API / agent selection
- System prompt, temperature, tools, MCP
- Persistent memory beyond the ACP connection/session
- Auth / multi-user
- Flutter client changes (beyond README usage notes if needed)
- Echo fallback when env is unset
- Non-OpenAI providers in this slice

## Approach

**Chosen:** Thin `ChatStreamer` provider interface + OpenAI HTTP SSE client; session transcript owned by `runtime`; agent stays ACP-facing.

**Rejected for this slice:**

- Call OpenAI directly inside `Agent.Prompt` with no provider boundary — harder to fake and replace when catalog/definitions arrive.
- Full LLM/agent frameworks — more surface than needed to prove the path.

## Architecture

```text
Flutter / acp-cli
  → WS /acp
  → ACP Agent (Go)
  → runtime.Session (id + definition + transcript[])
  → provider.OpenAI.StreamChat(messages)
  → Unsloth Studio (OpenAI-compatible /v1/chat/completions, stream)
  → session/update agent_message_chunk (per delta)
  → PromptResponse end_turn
```

### Components

| Piece | Responsibility |
| --- | --- |
| `cmd/controlplane` | Load env config at startup; wire OpenAI provider into agent/runtime; exit if incomplete |
| `internal/config` | Read `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`; trim trailing `/` on base URL |
| `internal/runtime` | Session map gains `Messages []{Role, Content}`; append user before inference, assistant after a completed stream |
| `internal/provider` | `ChatStreamer` interface + OpenAI HTTP SSE client; parse `data:` deltas; invoke `onDelta(text)` |
| `internal/agent` | `Prompt` / `Cancel` orchestrate history + stream + ACP updates |
| Echo | Removed from the live path; tests use a fake `ChatStreamer` / `httptest` OpenAI instead |

### Config

Required environment variables (no flags, no config file in this slice):

| Variable | Purpose |
| --- | --- |
| `OPENAI_BASE_URL` | API root, e.g. `http://127.0.0.1:8000/v1` (Unsloth Studio OpenAI-compatible base) |
| `OPENAI_API_KEY` | Bearer token (Unsloth may accept any non-empty value) |
| `OPENAI_MODEL` | Model id sent in Chat Completions requests |

Missing any → process exits before listen, with a clear error naming the variable.

Request URL: `{OPENAI_BASE_URL}/chat/completions` (after normalizing trailing slash).

### Runtime flow

1. Client opens WebSocket to `/acp`; `initialize`; `session/new` (still pins a single hardcoded definition snapshot; provider is process-global for this slice).
2. `session/prompt` — extract user text; append `{role: user, content}` to session transcript.
3. Provider POSTs full transcript as `messages` with `stream: true`, `Authorization: Bearer …`, and configured `model`.
4. Each content delta → `session/update` (`agent_message_chunk`).
5. On successful stream end — append `{role: assistant, content: fullText}` to transcript; return `PromptResponse{StopReason: end_turn}`.
6. Further prompts reuse the same session id and full history until disconnect.
7. WebSocket close — drop connection-local sessions (including transcript), same as today.

No system message is injected in this slice.

### Errors and cancel

- OpenAI HTTP/SSE failure mid-turn → stop streaming; return an ACP error for the prompt. User message remains in history; do not append a partial assistant message.
- Malformed SSE or empty completed assistant text → provider error; do not append an empty assistant turn.
- `session/cancel` → cancel the request context; abort the HTTP body; do not commit partial assistant text to history.

## Repo layout (deltas)

```text
controlplane/
  cmd/controlplane/          # load config; wire OpenAI streamer
  internal/config/           # new: env loader
  internal/provider/         # ChatStreamer + openai stream client; echo removed from live path
  internal/runtime/          # session transcript
  internal/agent/            # Prompt/Cancel use streamer + history
```

## Verification

**Manual (Unsloth)**

```text
export OPENAI_BASE_URL=http://127.0.0.1:<unsloth-port>/v1
export OPENAI_API_KEY=sk-local
export OPENAI_MODEL=<model-id>
go -C controlplane run ./cmd/controlplane
# Flutter or acp-cli: send a prompt; expect streamed model text
# Second prompt in same session should reflect prior turns
```

**Automated**

- Unit: OpenAI SSE parser/streamer against `httptest` fixture.
- Unit: runtime transcript append order.
- Integration: in-process ACP server + fake OpenAI that streams chunks; assert initialize → session/new → prompt → multiple updates → end_turn; second prompt’s outbound `messages` include prior user/assistant turns.
- `go test ./...` with no real Unsloth or external network.

## Success criteria

1. With env set and Unsloth up, Flutter chat receives a streamed model reply (not echo).
2. Multi-turn in one session: second prompt’s Chat Completions payload includes prior user and assistant text.
3. `go test ./...` passes offline with fakes.
4. Startup without required env fails clearly before listen.
5. Cancel aborts in-flight generation and leaves no partial assistant turn in history.

## Follow-ups (explicitly later)

- Definition-scoped provider/model/system prompt via catalog
- Temperature and other sampling params
- Tools / MCP
- Persistent memory beyond the ACP session
- Alternate providers behind the same `ChatStreamer` interface
