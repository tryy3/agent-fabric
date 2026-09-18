# Sandbox Tool Calling (E2E) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire CWD `sandbox.json` file tools into Chat Completions + an agent tool loop, emit ACP tool_call updates with raw input/output, and show collapsible tool bubbles in Flutter (thinking pattern) with history persistence.

**Architecture:** Provider sends OpenAI `tools` and accumulates streamed `tool_calls`; Agent opens a sandbox env per prompt (session-scoped), runs a bounded tool loop, and emits `StartToolCall`/`UpdateToolCall`. Flutter upserts `ChatBubbleKind.toolCall` bubbles; catalog `parts` gain `tool_call` entries for reload.

**Tech Stack:** Go 1.22+, `github.com/coder/acp-go-sdk@v0.13.5`, existing OpenAI SSE client, Flutter/`acpd` SessionUpdate tool_call kinds, existing sandbox registry.

## Global Constraints

- Worktree: `/home/tryy3/src/agent-fabric/.worktrees/sandbox-fs-tools` on `feat/sandbox-fs-tools`.
- Spec: `docs/superpowers/specs/2026-09-16-sandbox-tool-calling-design.md`.
- No Settings UI for per-agent tools; every agent uses tools from CWD `sandbox.json`.
- No `session/request_permission` in this POC — auto-execute sandbox file tools.
- Max **8** tool rounds per Prompt.
- Committed LLM hydrate for *next* Prompt: user + assistant **visible text only** (tool I/O in `parts` for UI; in-loop OpenAI tool messages only within the current Prompt).
- Provider must **not** import `internal/sandbox` (define tool schema types in `provider` or pass `json.RawMessage`).
- CI hermetic: fake streamer; no live Unsloth.
- TDD; commit per task.
- Go tests: `nix develop -c go -C controlplane test …`; Flutter: `cd client && flutter test …`.

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/runtime/session.go` | Extend `Message` for tool_calls / tool role |
| `controlplane/internal/provider/streamer.go` | `ToolDefinition`, `StreamChatOptions`, tool events on `StreamEvent` |
| `controlplane/internal/provider/openai.go` | Marshal tools; parse tool_calls deltas |
| `controlplane/internal/provider/openai_test.go` | SSE tool_calls fixtures |
| `controlplane/internal/catalog/thread_types.go` | `MessagePart` tool_call fields |
| `controlplane/internal/agent/agent.go` | Sandbox opts, tool loop, ACP tool updates, parts |
| `controlplane/internal/agent/agent_test.go` | Fake streamer multi-round + tools |
| `controlplane/internal/transport/ws/handler.go` | Pass sandbox opts into `agent.New` |
| `controlplane/cmd/controlplane/main.go` | Thread opts into server/WS |
| `controlplane/internal/server/server.go` | Plumb opts |
| `client/lib/acp/agent_connection.dart` | Parse tool_call updates → events |
| `client/lib/chat/chat_bubble.dart` | `ChatBubbleKind.toolCall` + fields |
| `client/lib/chat/chat_controller.dart` | Upsert by toolCallId |
| `client/lib/chat/agent_bubble.dart` | `_ToolCallActivity` collapsible UI |
| `client/lib/catalog/models.dart` | Hydrate `tool_call` parts |
| Tests under `client/test/…` | Connection + bubble + hydrate |

---

### Task 1: runtime.Message tool protocol

**Files:**
- Modify: `controlplane/internal/runtime/session.go`
- Modify: `controlplane/internal/runtime/session_test.go`

**Interfaces:**
- Produces:
```go
type ToolCall struct {
    ID       string `json:"id"`
    Type     string `json:"type"` // "function"
    Function ToolCallFunction `json:"function"`
}
type ToolCallFunction struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}
type Message struct {
    Role       string     `json:"role"`
    Content    string     `json:"content,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
    Name       string     `json:"name,omitempty"`
}
```

- [ ] **Step 1: Write failing test** asserting JSON marshal of assistant+tool_calls and tool role messages omit empty content correctly when encoded with `encoding/json`.

- [ ] **Step 2: Run** `nix develop -c go -C controlplane test ./internal/runtime/ -run Tool -count=1` — expect FAIL.

- [ ] **Step 3: Extend `Message`** as above (keep existing Role/Content working).

- [ ] **Step 4: Run tests — PASS.**

- [ ] **Step 5: Commit** `feat(runtime): extend Message for OpenAI tool_calls`

---

### Task 2: Provider tools + SSE tool_calls

**Files:**
- Modify: `controlplane/internal/provider/streamer.go`
- Modify: `controlplane/internal/provider/openai.go`
- Modify: `controlplane/internal/provider/openai_test.go`
- Modify: all `ChatStreamer` implementers/fakes (`echo` if any, agent test fakes)

**Interfaces:**
- Produces:
```go
type ToolDefinition struct {
    Type     string `json:"type"`
    Function struct {
        Name        string          `json:"name"`
        Description string          `json:"description"`
        Parameters  json.RawMessage `json:"parameters"`
    } `json:"function"`
}
type ToolCall struct { // completed call for the agent
    ID        string
    Name      string
    Arguments string // raw JSON object string
}
type StreamEvent struct {
    Thought   string
    Content   string
    Finish    string
    Usage     *Usage
    ToolCalls []ToolCall // set when a tool_calls turn completes (finish_reason tool_calls)
}
type StreamChatOptions struct {
    Tools []ToolDefinition
}
// ChatStreamer.StreamChat(ctx, model, messages, opts StreamChatOptions, onEvent) error
```

- [ ] **Step 1: Failing test** — httptest SSE fixture with fragmented `delta.tool_calls` then `finish_reason: tool_calls`; assert `onEvent` receives `ToolCalls` with merged arguments; assert request body includes `"tools"`.

- [ ] **Step 2: Run test — FAIL.**

- [ ] **Step 3: Implement** request `Tools` field; accumulate tool call deltas by index; on finish_reason `tool_calls`, emit event with completed calls (and still set `Finish`). Do not require content for tool-only turns.

- [ ] **Step 4: Update all ChatStreamer call sites/fakes** to new signature (pass empty opts where unused).

- [ ] **Step 5: Run** `nix develop -c go -C controlplane test ./internal/provider/ ./internal/agent/ ./internal/server/ ./internal/transport/... -count=1` — PASS.

- [ ] **Step 6: Commit** `feat(provider): stream OpenAI tools and tool_calls`

---

### Task 3: Catalog MessagePart tool_call

**Files:**
- Modify: `controlplane/internal/catalog/thread_types.go`
- Modify: `controlplane/internal/catalog/threads_store_test.go` (or new test)

**Interfaces:**
- Extend `MessagePart`:
```go
ToolCallID string `json:"toolCallId,omitempty"`
Name       string `json:"name,omitempty"`
Title      string `json:"title,omitempty"`
Input      string `json:"input,omitempty"`  // raw JSON string
Output     string `json:"output,omitempty"` // raw JSON string
Status     string `json:"status,omitempty"` // pending|completed|failed
```
- `Type: "tool_call"` uses these fields; `Text` optional summary.

- [ ] **Step 1–4: TDD** store/load round-trip via `CommitTurn` + get thread messages.

- [ ] **Step 5: Commit** `feat(catalog): persist tool_call message parts`

---

### Task 4: Agent tool loop + ACP + wire sandbox opts

**Files:**
- Modify: `controlplane/internal/agent/agent.go`
- Modify: `controlplane/internal/agent/agent_test.go`
- Modify: `controlplane/cmd/controlplane/main.go`
- Modify: `controlplane/internal/server/server.go`
- Modify: `controlplane/internal/transport/ws/handler.go`

**Interfaces:**
- `agent.New(store, catalog, sandboxOpts sandbox.OpenOptions)` (or `*sandbox.OpenOptions`; zero Kind ⇒ no tools).
- Helper `sandboxTools(env) (registry, []provider.ToolDefinition)`.
- `turnParts` accepts ordered tool parts collected during the loop.
- ACP: `acp.StartToolCall` / `acp.UpdateToolCall` with RawInput/RawOutput (check SDK helpers in module cache).

**Loop sketch:**
1. Open env from opts (+ SessionID); defer Close.
2. If no FS / no defs → single StreamChat as today.
3. Else loop ≤8: StreamChat with tools; if `ev.ToolCalls` non-empty (or Finish==tool_calls): for each call emit Start, `registry.Call`, emit Update, append messages; continue. Else treat content as final.
4. Allow empty final content only if… keep requiring final assistant text for CommitTurn content (if model ends on tools only, do one more stream without forcing — if still empty, error as today).
5. Build parts: thoughts (concat per turn or per round — simplest: all thought text then each tool_call part in order then message then usage).

- [ ] **Step 1: Failing agent test** with fake streamer: round1 returns tool_calls `read_file`; after tool result, round2 returns content `"ok"`. Assert `registry` called; assert fake ACP conn recorded Start+Update; assert CommitTurn parts include `tool_call` with input/output when thread-bound (use temp catalog DB like existing tests).

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement** loop + plumbing from main → server → ws → agent.

- [ ] **Step 4: Run** `nix develop -c go -C controlplane test ./internal/agent/ ./internal/server/ ./internal/transport/... ./internal/sandboxconfig/ -count=1` — PASS.

- [ ] **Step 5: Commit** `feat(agent): sandbox tool loop with ACP tool_call updates`

---

### Task 5: Flutter ACP tool events + bubbles

**Files:**
- Modify: `client/lib/acp/agent_connection.dart`
- Modify: `client/lib/chat/chat_bubble.dart`
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/lib/catalog/models.dart`
- Create/Modify tests under `client/test/`

**Interfaces:**
- `AgentToolCallEvent({required id, title, status, rawInput, rawOutput, inProgress})` on sealed turn events.
- `ChatBubbleKind.toolCall` with `toolCallId`, `toolTitle`, `toolStatus`, `toolInput`, `toolOutput`, `streamingTool`.
- Controller: on event, find bubble by id or append; merge fields; `streamingTool: status != completed/failed`.
- `_ToolCallActivity`: ExpansionTile/custom like `_ThoughtActivity`; expanded children show labeled Input/Output (`Select`/`Text` monospace); header `toolTitle` + short status.
- `bubblesFromThreadMessage`: for each part type `tool_call`, emit a toolCall bubble in order (before message).

- [ ] **Step 1: Widget/controller tests** — upsert two updates same id; hydrate from JSON parts.

- [ ] **Step 2: flutter test FAIL.**

- [ ] **Step 3: Implement UI + connection parsing** (use `acpd` SessionUpdate variants — inspect package for exact type names).

- [ ] **Step 4: `cd client && flutter test` relevant files — PASS.

- [ ] **Step 5: Commit** `feat(client): collapsible tool-call bubbles with input/output`

---

### Task 6: README smoke notes

**Files:**
- Modify: `README.md` sandbox section

- [ ] Document: run from dir with `sandbox.json`; ask the agent to read a mounted file; expect Thinking-like tool bubble with input/output; tools not configurable in Settings yet.

- [ ] Commit `docs: describe end-to-end sandbox tool calling smoke`

---

## Spec coverage

| Spec item | Task |
| --- | --- |
| OpenAI tools + tool_calls parse | 2 |
| Message tool protocol | 1 |
| Agent loop + SessionID + registry | 4 |
| ACP Start/Update raw I/O | 4 |
| Persist tool_call parts | 3, 4 |
| Flutter collapsible + I/O | 5 |
| No Settings tool config | all |
| README | 6 |

## Placeholder scan

Avoid TBD steps; SDK helper names must be verified against `acp-go-sdk@v0.13.5` / `acpd` during Task 4/5 (open module source once, use exact helpers).
