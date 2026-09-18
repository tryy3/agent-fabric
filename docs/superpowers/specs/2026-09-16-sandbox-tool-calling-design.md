# End-to-end sandbox tool calling (POC)

**Date:** 2026-09-16  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-15-sandbox-fs-tools-design.md](./2026-09-15-sandbox-fs-tools-design.md), [2026-09-14-chat-transparency-design.md](./2026-09-14-chat-transparency-design.md), CWD `sandbox.json` load in `cmd/controlplane`

## Problem

The sandbox package exposes `read_file` / `write_file` with OpenAI-shaped schemas, and the control plane loads `sandbox.json` from CWD, but the LLM never receives `tools`, never emits `tool_calls`, and the Flutter cockpit has no tool UI. Transparency requires the user to see tool **input and output** on the same path as thinking (collapsible activity), live and after refresh.

The sandbox V1 plan intentionally deferred this wiring; this slice closes that gap for a fixed tool set from config (not per-agent UI configuration).

## Goals

- End-to-end: user prompt → model may call `read_file` / `write_file` → plane executes in the configured sandbox → results return to the model → final answer streams to the client.
- ACP carries tool lifecycle with **raw input and raw output** (and human-readable content) so clients are not guessing from text.
- Flutter shows each tool call as its own **collapsible container** (same interaction pattern as Thinking), with expanded body showing Input + Output.
- Persist tool calls in thread `parts` so history reload matches the live turn order (thought / tool / message / usage).
- Use `sandbox.json` from CWD for every agent in this POC (no Settings tool picker).
- Hermetic tests: fake streamer with scripted tool_calls; no live Unsloth required in CI.

## Non-goals

- Configuring tools / MCP / sandbox per agent in the Settings UI.
- Permission prompts per tool call (`session/request_permission`) — auto-run sandbox file tools in this POC.
- Terminal / execute tool, MCP tools, client-origin tools.
- Showing tool transcripts to the **model** on later turns beyond the normal OpenAI tool-message history for the **current** multi-round prompt (committed thread LLM hydrate stays user/assistant visible text only, same as thoughts).
- Hot-reload of `sandbox.json` mid-process.
- Flutter visibility settings UI polish beyond a thinking-like default (collapsed when idle; open while in progress). Optional Settings row is nice-to-have, not required if we hardcode the same defaults as thinking.

## Approach

**Chosen:** Real ACP `tool_call` / `tool_call_update` + collapsible bubbles (Approach B).

**Rejected:**

- Dump tool I/O into `agent_message_chunk` — breaks transparency and ordered parts.
- Full permission UI per call — out of POC scope.

## Architecture

```text
sandbox.json (CWD) → OpenOptions on Agent
                         │
session/prompt ──────────┤
                         ▼
              Open env (SessionID=sess.ID)
              Registry + Definitions(env)
                         │
         ┌───────────────┴───────────────┐
         ▼                               ▼
  Chat Completions                 ACP SessionUpdate
  tools + messages                 StartToolCall (rawInput)
         │                         UpdateToolCall (rawOutput)
         ▼                               │
  tool_calls? ──yes──► Call tools ───────┘
         │ no
         ▼
  agent_message_chunk / thought / usage
         │
         ▼
  CommitTurn parts: thought* | tool_call* | message | usage
```

### Server components

| Piece | Change |
| --- | --- |
| `runtime.Message` | Support OpenAI chat tool protocol: optional `ToolCalls`, `ToolCallID`, `Name`; `Content` may be empty for assistant tool-call turns |
| `provider.ChatStreamer` | Accept tools; emit tool-call stream events (accumulated by id/index); finish reason `tool_calls` |
| `provider.OpenAI` | Marshal `tools`; parse `delta.tool_calls` / `finish_reason` |
| `agent.Agent` | Hold `sandbox.OpenOptions`; tool loop; ACP Start/UpdateToolCall; persist tool parts |
| `cmd/controlplane` | Pass loaded opts into `agent.New` / server / WS handler |
| `catalog` parts | New part type `tool_call` with id, name, title, input, output, status |

### Tool loop (per Prompt)

1. Clone opts; if docker session scope, set `Scope.SessionID = sess.ID`.
2. `env, err := sandbox.Open(ctx, opts)`; `defer env.Close`.
3. Register `file.Tools()`; `defs := registry.Definitions(env)`.
4. Messages = history (text-only hydrate as today) + new user message.
5. Loop (max **8** rounds):
   - `StreamChat(..., messages, defs, onEvent)`
   - On text/thought/usage: existing ACP behavior
   - On completed tool_calls: for each call, ACP pending with `rawInput`, `Registry.Call`, ACP completed with `rawOutput` / text content; append assistant message with tool_calls + tool role messages; continue
   - On final assistant text (no tool_calls): break
6. Commit turn with ordered parts including tool_call entries in execution order relative to thoughts/message as observed.

If `defs` is empty (env without FS), behave as today (no tools field).

### ACP mapping

| Moment | ACP |
| --- | --- |
| Tool requested | `StartToolCall(id, title, kind=read\|edit, status=pending, rawInput=args JSON, locations if path known)` |
| Tool finished | `UpdateToolCall(id, status=completed\|failed, rawOutput=result JSON, content=[text summary])` |

Titles: e.g. `Read file` / `Write file`. Kind: `read_file` → `ToolKindRead`, `write_file` → `ToolKindEdit` (or equivalent SDK constants).

### Flutter

| Piece | Change |
| --- | --- |
| `agent_connection` | Handle `tool_call` / `tool_call_update` → `AgentToolCallEvent` (id, title, status, rawInput, rawOutput, streaming) |
| `ChatBubbleKind` | Add `toolCall` |
| `chat_controller` | Upsert bubble by `toolCallId` (merge updates; do not append like thought text) |
| `AgentBubble` | `_ToolCallActivity` — collapsible; header title + status line; body **Input** / **Output** sections (pretty-print JSON strings) |
| History | `ThreadMessage` / `bubblesFromThreadMessage` hydrate `parts` type `tool_call` into bubbles in order |
| Visibility | Same pattern as thinking: expanded while in-progress; else default collapsed (hardcode or reuse/add pref) |

### LLM history vs UI history

- **Within a single Prompt tool loop:** full OpenAI tool messages are sent back to the model.
- **Committed thread hydrate for the next Prompt:** keep sending **user + assistant visible text only** to the model (same rule as thoughts). Tool I/O remains in `parts` for the UI only. (If a follow-up needs cross-turn tool memory, that is a later design.)

## Success criteria

1. With docker/local `sandbox.json`, asking “read `/workspace/test.json`” (or mounted path) produces a tool call the model chooses, ACP updates with input args and output body, Flutter shows a collapsible tool bubble with both, and a final answer.
2. Refresh / reopen thread restores tool bubbles from `parts`.
3. Unit/fake tests cover: tools on request body; tool_calls stream accumulation; agent loop Call + second StreamChat; ACP helpers invoked (fake conn); Flutter widget/controller tests for upsert + collapse.
4. No Settings UI for tool lists.
5. CI stays offline (no real provider required).

## Open follow-ups

- Per-agent tool allowlists in catalog UI
- `session/request_permission` for destructive tools
- Cross-turn tool results in model context
- Terminal tool on `Executor`
