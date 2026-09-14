# Chat Transparency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stream Unsloth thinking and generation stats over ACP, show them in Flutter behind collapsed expanders, and persist the same turn parts so refresh restores them.

**Architecture:** The OpenAI streamer emits `StreamEvent` values (thought, content, usage). The ACP agent maps those to `agent_thought_chunk`, `agent_message_chunk`, and `usage_update`. `CommitTurn` stores visible `content` plus JSON `parts` and model/provider/stopReason. Flutter renders live updates and HTTP history with the same widgets. LLM hydrate still uses `{role, content}` text only.

**Tech Stack:** Go 1.22+, pgx/sqlc/goose, `github.com/coder/acp-go-sdk` v0.13.5 (`UpdateAgentThoughtText`, `SessionUsageUpdate`), Flutter, `acpd` 1.0.0, `shared_preferences`.

## Global Constraints

- Spec: [`2026-09-14-chat-transparency-design.md`](../specs/2026-09-14-chat-transparency-design.md).
- No `sent` parts, tools, MCP, plans, permission UI, or per-chat visibility override.
- Cancel / stream error / thinking-without-content writes **no** message rows.
- Next LLM request is user/assistant **visible text only** (never thought or usage).
- Caption `model · provider · tok/s` is always visible; tok/s omitted when unknown.
- Thinking/Stats default **collapsed**; Settings can set collapsed / expanded / hidden. `hidden` hides the expander; data is still persisted.
- ACP Go `SessionUsageUpdate.Used` and `Size` are `int` (not pointers): send `0` when unknown; put real timings in `_meta`.
- `_meta` keys (lock these): `ttftMs`, `elapsedMs`, `deltas`, `promptMs`, `predictedMs`, `promptPerSecond`, `predictedPerSecond`, `promptTokens`, `completionTokens`, `totalTokens`, `stopReason`.
- `include_usage: true` on Chat Completions. Final `choices: []` + `usage` is success, not empty assistant.
- Module path `github.com/tryy3/agent-fabric` (root `controlplane/`).
- Follow TDD: failing test → implement → pass → commit per task.
- sqlc from `controlplane/internal/db` (`sqlc generate`). Next migration is `00004_message_parts`.

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/provider/streamer.go` | `StreamEvent`, `Usage`, `ChatStreamer` |
| `controlplane/internal/provider/openai.go` | SSE parse: thought, content, usage, timings, finish_reason |
| `controlplane/internal/db/migrations/00004_message_parts.{up,down}.sql` | `parts` jsonb + assistant metadata columns |
| `controlplane/internal/db/queries/threads.sql` | Insert/list messages with new columns |
| `controlplane/internal/db/queries/agents.sql` | List/get agents with `provider_name` join |
| `controlplane/internal/catalog/thread_types.go` | `MessagePart`, `AssistantTurn`, HTTP fields |
| `controlplane/internal/catalog/store.go` | `CommitTurn` with parts; agent `providerName` |
| `controlplane/internal/runtime/definition.go` | `SessionPin.ProviderName` |
| `controlplane/internal/agent/agent.go` | Thought/usage ACP updates; commit parts |
| `client/lib/acp/agent_connection.dart` | `AgentTurnEvent` union; forward thought/usage |
| `client/lib/chat/chat_message.dart` | Thought, caption, usage on assistant turns |
| `client/lib/chat/chat_controller.dart` | Accumulate live turn; map HTTP parts |
| `client/lib/chat/assistant_turn.dart` | Bubble + caption + expanders |
| `client/lib/chat/display_settings.dart` | Visibility prefs |
| `client/lib/settings/chat_tab.dart` | Settings → Chat |
| `client/lib/settings/settings_page.dart` | Third tab |
| `client/lib/catalog/models.dart` | `providerName`, message parts |
| `README.md` | Thinking / stats UX note |

**Interfaces this plan adds** (later tasks consume these names exactly):

```go
// provider
type StreamEvent struct {
	Thought string
	Content string
	Finish  string // OpenAI finish_reason for this chunk, may be empty
	Usage   *Usage
}

type Usage struct {
	PromptTokens       *int
	CompletionTokens   *int
	TotalTokens        *int
	PromptMs           *float64
	PredictedMs        *float64
	PromptPerSecond    *float64
	PredictedPerSecond *float64
	TTFTMs             *int64
	ElapsedMs          *int64
	Deltas             int
}

type ChatStreamer interface {
	StreamChat(ctx context.Context, model string, messages []runtime.Message, onEvent func(StreamEvent) error) error
}

// catalog
type MessagePart struct {
	Type               string   `json:"type"`
	Text               string   `json:"text,omitempty"`
	PromptTokens       *int     `json:"promptTokens,omitempty"`
	CompletionTokens   *int     `json:"completionTokens,omitempty"`
	TotalTokens        *int     `json:"totalTokens,omitempty"`
	ContextUsed        *int     `json:"contextUsed,omitempty"`
	ContextSize        *int     `json:"contextSize,omitempty"`
	PromptMs           *float64 `json:"promptMs,omitempty"`
	PredictedMs        *float64 `json:"predictedMs,omitempty"`
	TTFTMs             *int64   `json:"ttftMs,omitempty"`
	ElapsedMs          *int64   `json:"elapsedMs,omitempty"`
	PromptPerSecond    *float64 `json:"promptPerSecond,omitempty"`
	PredictedPerSecond *float64 `json:"predictedPerSecond,omitempty"`
	Deltas             *int     `json:"deltas,omitempty"`
}

type AssistantTurn struct {
	Content      string
	Model        string
	ProviderID   string
	ProviderName string
	StopReason   string
	Parts        []MessagePart
}

func (s *Store) CommitTurn(ctx context.Context, threadID, userText string, assistant AssistantTurn) (Thread, error)
```

```dart
sealed class AgentTurnEvent {
  const AgentTurnEvent();
}

final class AgentThoughtDelta extends AgentTurnEvent {
  const AgentThoughtDelta(this.text);
  final String text;
}

final class AgentMessageDelta extends AgentTurnEvent {
  const AgentMessageDelta(this.text);
  final String text;
}

final class AgentUsageEvent extends AgentTurnEvent {
  const AgentUsageEvent(this.usage);
  final TurnUsage usage;
}

class TurnUsage {
  const TurnUsage({
    this.promptTokens,
    this.completionTokens,
    this.totalTokens,
    this.ttftMs,
    this.elapsedMs,
    this.promptMs,
    this.predictedMs,
    this.promptPerSecond,
    this.predictedPerSecond,
    this.deltas,
    this.stopReason,
  });
  final int? promptTokens;
  final int? completionTokens;
  final int? totalTokens;
  final int? ttftMs;
  final int? elapsedMs;
  final double? promptMs;
  final double? predictedMs;
  final double? promptPerSecond;
  final double? predictedPerSecond;
  final int? deltas;
  final String? stopReason;
}

typedef AgentTurnHandler = void Function(AgentTurnEvent event);

abstract class AgentSessionApi {
  Future<void> sendPrompt(String text, {required AgentTurnHandler onEvent});
  // other methods unchanged
}

enum VisibilityMode { collapsed, expanded, hidden }
```

---

### Task 1: StreamEvent + OpenAI SSE extras

**Files:**
- Modify: `controlplane/internal/provider/streamer.go`
- Modify: `controlplane/internal/provider/openai.go`
- Modify: `controlplane/internal/provider/openai_test.go`
- Modify: `controlplane/internal/agent/agent.go` (callback signature only; still emit message chunks)
- Modify: `controlplane/internal/agent/agent_test.go` (fake `StreamChat` signatures)

**Interfaces:**
- Consumes: existing OpenAI SSE loop
- Produces: `StreamEvent`, `Usage`, `ChatStreamer.StreamChat(..., func(StreamEvent) error)`

- [ ] **Step 1: Write failing tests for thought, usage chunk, include_usage, thinking-only error**

Add to `controlplane/internal/provider/openai_test.go`. Keep existing tests compiling by switching their callbacks to `func(provider.StreamEvent) error` and collecting `ev.Content` — those will fail to compile until Step 3, which is the TDD signal together with the new tests.

New tests (write these first in the same file):

```go
func TestOpenAIIncludeUsageAndReasoning(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hmm\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1,\"total_tokens\":4},\"timings\":{\"prompt_ms\":10,\"predicted_ms\":20,\"prompt_per_second\":100.5,\"predicted_per_second\":40.25}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk-test", srv.Client())
	var thoughts, contents []string
	var usage *provider.Usage
	var finish string
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "q"}}, func(ev provider.StreamEvent) error {
		if ev.Thought != "" {
			thoughts = append(thoughts, ev.Thought)
		}
		if ev.Content != "" {
			contents = append(contents, ev.Content)
		}
		if ev.Finish != "" {
			finish = ev.Finish
		}
		if ev.Usage != nil {
			usage = ev.Usage
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	opts, _ := gotBody["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Fatalf("stream_options = %#v", gotBody["stream_options"])
	}
	if strings.Join(thoughts, "") != "hmm" || strings.Join(contents, "") != "hi" {
		t.Fatalf("thoughts=%v contents=%v", thoughts, contents)
	}
	if finish != "stop" {
		t.Fatalf("finish = %q", finish)
	}
	if usage == nil || usage.PromptTokens == nil || *usage.PromptTokens != 3 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.PredictedPerSecond == nil || *usage.PredictedPerSecond != 40.25 {
		t.Fatalf("tok/s = %+v", usage.PredictedPerSecond)
	}
	if usage.Deltas != 1 {
		t.Fatalf("deltas = %d", usage.Deltas)
	}
	if usage.TTFTMs == nil || usage.ElapsedMs == nil {
		t.Fatal("expected plane TTFT and elapsed")
	}
}

func TestOpenAIThinkingWithoutContentIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"only\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	client := provider.NewOpenAI(srv.URL+"/v1", "sk", srv.Client())
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "x"}}, func(provider.StreamEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want empty", err)
	}
}
```

Update existing `StreamChat` callbacks in this file from `func(delta string)` / `func(string)` to `func(ev provider.StreamEvent)` collecting `ev.Content`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/provider/ -count=1`

Expected: FAIL compile (`StreamEvent` undefined and/or `onDelta func(string)` mismatch).

- [ ] **Step 3: Implement StreamEvent and parser**

`streamer.go` — replace `ChatStreamer` with the types in **Interfaces** above.

`openai.go`:

- `chatRequest` gains `StreamOptions *streamOptions \`json:"stream_options,omitempty"\`` with `IncludeUsage bool \`json:"include_usage"\``. Always set `{IncludeUsage: true}`.
- `streamChunk` parses:

```go
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Timings *struct {
		PromptMs           float64 `json:"prompt_ms"`
		PredictedMs        float64 `json:"predicted_ms"`
		PromptPerSecond    float64 `json:"prompt_per_second"`
		PredictedPerSecond float64 `json:"predicted_per_second"`
	} `json:"timings"`
}
```

- After HTTP 200, `streamStart := time.Now()`. First thought or content sets `ttft`. Count `Deltas` as content events only.
- For each SSE JSON object: if thought non-empty → `onEvent(StreamEvent{Thought, Finish})`; if content non-empty → `onEvent(StreamEvent{Content, Finish})`; if `usage != nil` remember it (do not treat empty choices as failure).
- After the loop, if no content → `empty assistant response`.
- Then `onEvent` once with `Usage` populated: provider token/timing pointers only when that object was present (do not point at `0` just because the field was missing). Always set `TTFTMs`, `ElapsedMs`, `Deltas`.
- Helper `ptrInt(v int) *int` / `ptrF64` only when the field existed on the JSON.

Update `agent.go` Prompt callback to `func(ev provider.StreamEvent) error` and only handle `ev.Content` (ignore thought/usage until Task 3).

Update `fakeStreamer` / `recordingStreamer` / `streamFn` in `agent_test.go` to the new signature. For `deltas []string`, emit `StreamEvent{Content: d}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/provider/ ./internal/agent/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/provider controlplane/internal/agent
git commit -m "feat(provider): parse thought, usage, and timings from OpenAI streams"
```

---

### Task 2: Persist parts + providerName

**Files:**
- Create: `controlplane/internal/db/migrations/00004_message_parts.up.sql`
- Create: `controlplane/internal/db/migrations/00004_message_parts.down.sql`
- Modify: `controlplane/internal/db/queries/threads.sql`
- Modify: `controlplane/internal/db/queries/agents.sql`
- Modify: generated `controlplane/internal/db/*.go` via `sqlc generate`
- Modify: `controlplane/internal/catalog/thread_types.go`
- Modify: `controlplane/internal/catalog/types.go` (`Agent.ProviderName`)
- Modify: `controlplane/internal/catalog/store.go`
- Modify: `controlplane/internal/catalog/threads_store_test.go`
- Modify: `controlplane/internal/catalog/threads_http_test.go`
- Modify: `controlplane/internal/catalog/http.go` (JSON already uses structs)
- Modify: `controlplane/internal/catalog/store_test.go` (agent JSON `providerName`)
- Modify: `controlplane/internal/agent/agent.go` (`CommitTurn` new signature)
- Modify: `controlplane/internal/agent/agent_test.go` (`CommitTurn` call sites)
- Test: `controlplane/internal/db/threads_schema_test.go` (add parts insert)

**Interfaces:**
- Consumes: Task 1 module still compiling
- Produces: `MessagePart`, `AssistantTurn`, `CommitTurn(..., AssistantTurn)`, `Agent.ProviderName *string`

- [ ] **Step 1: Write failing schema + store tests**

Add to `threads_schema_test.go`:

```go
func TestMessagesPartsColumnDefaultsEmptyArray(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO threads (id, title, title_source, created_at, updated_at)
VALUES ('th_parts', 'Untitled', 'auto', now(), now())`)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO messages (id, thread_id, role, content, position, created_at)
VALUES ('msg_u', 'th_parts', 'user', 'hi', 0, now())`)
	if err != nil {
		t.Fatalf("insert without parts: %v", err)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT parts FROM messages WHERE id='msg_u'`).Scan(&raw); err != nil {
		t.Fatalf("select parts: %v", err)
	}
	if string(raw) != "[]" && string(raw) != "[]\n" {
		t.Fatalf("parts = %s, want []", raw)
	}
}
```

Add to `threads_store_test.go`:

```go
func TestCommitTurnStoresAssistantPartsAndMetadata(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pt, ps := 3, 40.25
	ttft := int64(12)
	deltas := 2
	_, err = store.CommitTurn(ctx, th.ID, "hi", catalog.AssistantTurn{
		Content:      "hello",
		Model:        "m1",
		ProviderID:   "prov_x",
		ProviderName: "Local",
		StopReason:   "end_turn",
		Parts: []catalog.MessagePart{
			{Type: "thought", Text: "hmm"},
			{Type: "message", Text: "hello"},
			{Type: "usage", PromptTokens: &pt, PredictedPerSecond: &ps, TTFTMs: &ttft, Deltas: &deltas},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	as := detail.Messages[1]
	if as.Role != "assistant" || as.Content != "hello" {
		t.Fatalf("assistant = %+v", as)
	}
	if as.Model == nil || *as.Model != "m1" || as.ProviderName == nil || *as.ProviderName != "Local" {
		t.Fatalf("meta = %+v", as)
	}
	if len(as.Parts) != 3 || as.Parts[0].Type != "thought" || as.Parts[0].Text != "hmm" {
		t.Fatalf("parts = %+v", as.Parts)
	}
	if detail.Messages[0].Parts == nil {
		t.Fatal("user parts must be [] not null")
	}
}
```

Add HTTP assertion in `TestThreadsHTTPCreateListGetRename` after the existing commit: decode `parts` on the assistant message (will fail until types exist). Or add `TestThreadsHTTPReturnsParts`.

Add agent test: create provider+agent, `GetAgent` / `ListAgents` include `ProviderName == "Local"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/db/ ./internal/catalog/ -count=1`

Expected: FAIL (`parts` column missing and/or `AssistantTurn` undefined).

- [ ] **Step 3: Migration, sqlc, store, HTTP**

`00004_message_parts.up.sql`:

```sql
ALTER TABLE messages
  ADD COLUMN parts jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN model text,
  ADD COLUMN provider_id text,
  ADD COLUMN provider_name text,
  ADD COLUMN stop_reason text;
```

`00004_message_parts.down.sql`:

```sql
ALTER TABLE messages
  DROP COLUMN IF EXISTS stop_reason,
  DROP COLUMN IF EXISTS provider_name,
  DROP COLUMN IF EXISTS provider_id,
  DROP COLUMN IF EXISTS model,
  DROP COLUMN IF EXISTS parts;
```

Update `InsertMessage` / `ListMessages` in `threads.sql` to include the new columns (user rows: `parts` `'[]'::jsonb`, metadata NULL).

`agents.sql` `ListAgents` and `GetAgent`:

```sql
SELECT
  a.id, a.name, a.description, a.version, a.provider_id, a.default_model,
  a.created_at, a.updated_at,
  p.name AS provider_name
FROM agents a
LEFT JOIN providers p ON p.id = a.provider_id
```

(`GetAgent` adds `WHERE a.id = $1`.) `ListAgentsByProvider` can stay agents-only; set `ProviderName` in Go when listing by provider if needed, or add the same join.

Run: `sqlc generate` in `controlplane/internal/db`.

`thread_types.go` — add `MessagePart`, `AssistantTurn`, and on `ThreadMessage`:

```go
Model        *string       `json:"model,omitempty"`
ProviderID   *string       `json:"providerId,omitempty"`
ProviderName *string       `json:"providerName,omitempty"`
StopReason   *string       `json:"stopReason,omitempty"`
Parts        []MessagePart `json:"parts"`
```

`Agent` gains `ProviderName *string \`json:"providerName,omitempty"\``.

`CommitTurn` signature becomes `(ctx, threadID, userText string, assistant AssistantTurn)`. User insert: `parts=[]`. Assistant insert: marshal `assistant.Parts` (if nil, write `[{type:message, text: Content}]` so old call sites that only set Content still persist a message part). Metadata pointers from non-empty strings.

Update every `CommitTurn` call site to `AssistantTurn{Content: "..."}`. `agent.go` Prompt: `catalog.AssistantTurn{Content: full.String()}`.

`GetThread` / `agentFromDB`: map new columns. Empty `parts` JSON → `[]MessagePart{}` not nil.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/db/ ./internal/catalog/ ./internal/agent/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db controlplane/internal/catalog controlplane/internal/agent
git commit -m "feat(catalog): persist assistant turn parts and providerName"
```

---

### Task 3: ACP thought/usage + commit snapshot

**Files:**
- Modify: `controlplane/internal/runtime/definition.go` (`ProviderName string`)
- Modify: `controlplane/internal/agent/agent.go`
- Modify: `controlplane/internal/agent/agent_test.go`
- Modify: `controlplane/internal/runtime/session_test.go` if pin literals need the new field (zero value is fine)

**Interfaces:**
- Consumes: `StreamEvent`, `CommitTurn(..., AssistantTurn)`, `SessionPin`
- Produces: ACP `UpdateAgentThoughtText`, `SessionUpdate{UsageUpdate}`, persisted parts from the live turn

- [ ] **Step 1: Write failing agent tests**

Extend `captureClient` to record thought texts and usage updates:

```go
thoughts []string
usages   []acp.SessionUsageUpdate

// in SessionUpdate:
if u.AgentThoughtChunk != nil && u.AgentThoughtChunk.Content.Text != nil {
    c.thoughts = append(c.thoughts, u.AgentThoughtChunk.Content.Text.Text)
}
if u.UsageUpdate != nil {
    c.usages = append(c.usages, *u.UsageUpdate)
}
```

Add `TestThoughtAndUsageOverACPAndCommit` (bound thread):

- `fakeStreamer.streamFn` emits `StreamEvent{Thought:"why "}`, `{Thought:"me"}`, `{Content:"hi", Finish:"stop"}`, `{Usage: &provider.Usage{PromptTokens: ptr(3), PredictedPerSecond: ptrF(35.5), TTFTMs: ptrI64(10), ElapsedMs: ptrI64(50), Deltas: 1}}`.
- After Prompt: `thoughts` join to `"why me"`, `chunks` to `"hi"`, `len(usages)==1`, `usages[0].Meta["predictedPerSecond"]` is 35.5, `usages[0].Meta["stopReason"]=="end_turn"`.
- `GET` thread (via catalog `GetThread`): assistant `Parts[0].Type=="thought"`, `Parts[1].Type=="message"`, `Parts[1].Text=="hi"`, `Model==pin.CurrentModel`, `ProviderName=="Local"`.
- Second prompt: `snapshotMessages()` assistant content is `"hi"` (no thought).

Add `TestMaxTokensStillCommits`: finish `length` → `StopReasonMaxTokens`, row `StopReason=="max_tokens"`.

`pinFromCatalog` must copy `ProviderName: p.Name` (assert in existing new-session test or the new one).

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/agent/ -run 'TestThoughtAndUsageOverACPAndCommit|TestMaxTokensStillCommits' -count=1`

Expected: FAIL (thoughts empty / parts missing).

- [ ] **Step 3: Implement Prompt mapping**

In `Prompt`, accumulate `thought` + `content` builders. On each event:

- Thought non-empty → `SessionUpdate{Update: acp.UpdateAgentThoughtText(ev.Thought)}`
- Content non-empty → existing `UpdateAgentMessageText`
- Remember last `Finish` and last `Usage`

After successful non-empty content:

1. Map finish: `length` → `acp.StopReasonMaxTokens`; `content_filter` → `acp.StopReasonRefusal`; otherwise `acp.StopReasonEndTurn`.
2. Build `Usage` if the streamer omitted it: `ElapsedMs` / `Deltas` / `TTFTMs` from this Prompt (start clock at stream begin). Prefer streamer `Usage` when present.
3. Emit one `usage_update`:

```go
used := 0
if u.TotalTokens != nil {
	used = *u.TotalTokens
}
upd := acp.SessionUpdate{UsageUpdate: &acp.SessionUsageUpdate{
	SessionUpdate: "usage_update",
	Used:          used,
	Size:          0,
	Meta:          usageMeta(u, stopReason),
}}
```

`usageMeta` copies the locked `_meta` keys, omitting nils.

4. `parts` order: thought (if any), message, usage. `message.Text == content`. Usage part copies the same numbers as `_meta`.
5. Bound `CommitTurn` with `AssistantTurn{Content, Model: sess.Pin.CurrentModel, ProviderID: sess.Pin.ProviderID, ProviderName: sess.Pin.ProviderName, StopReason: string(stopReason wire value: end_turn|max_tokens|refusal), Parts}`.
6. Unbound: append `runtime.Message{Role:"assistant", Content: content}` only (no thought).
7. Return `PromptResponse{StopReason: mapped}`.

`pinFromCatalog`: `ProviderName: p.Name`.

Stop reason strings persisted: `end_turn`, `max_tokens`, `refusal` (ACP wire values, not Go identifiers).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/agent/ ./internal/catalog/ ./internal/provider/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent controlplane/internal/runtime
git commit -m "feat(agent): stream thoughts and usage, persist turn parts"
```

---

### Task 4: Flutter ACP turn events

**Files:**
- Modify: `client/lib/acp/agent_connection.dart`
- Modify: `client/test/acp/agent_connection_test.dart`
- Modify: `client/test/chat/chat_controller_test.dart` (`FakeConn.sendPrompt`)
- Modify: `client/test/chat/chat_screen_test.dart`
- Modify: `client/test/app_shell_test.dart`

**Interfaces:**
- Consumes: ACP `AgentThoughtChunk`, `AgentMessageChunk`, `UsageSessionUpdate`
- Produces: `AgentTurnEvent`, `TurnUsage`, `AgentTurnHandler`, `sendPrompt(..., onEvent:)`

- [ ] **Step 1: Write failing connection tests**

In `agent_connection_test.dart`, keep `agentMessageText`. Add:

```dart
test('agentThoughtText extracts text from AgentThoughtChunk', () {
  expect(
    agentThoughtText(AgentThoughtChunk(
      chunk: ContentChunk(content: TextContentBlock(text: 'why')),
    )),
    'why',
  );
  expect(
    agentThoughtText(AgentMessageChunk(
      chunk: ContentChunk(content: TextContentBlock(text: 'x')),
    )),
    isNull,
  );
});

test('connect + prompt forwards thought, message, and usage', () async {
  // same linked-transport harness as the existing prompt test
  // onPrompt:
  ctx.sessionUpdate(sessionId: request.sessionId, update: AgentThoughtChunk(... 'why'));
  ctx.sessionUpdate(sessionId: request.sessionId, update: AgentMessageChunk(... 'hello'));
  ctx.sessionUpdate(
    sessionId: request.sessionId,
    update: UsageSessionUpdate(
      used: 4,
      meta: {'predictedPerSecond': 35.5, 'stopReason': 'end_turn', 'deltas': 1},
    ),
  );
  // collect onEvent into List<AgentTurnEvent>
  expect(events[0], isA<AgentThoughtDelta>());
  expect(events[1], isA<AgentMessageDelta>());
  expect(events[2], isA<AgentUsageEvent>());
  expect((events[2] as AgentUsageEvent).usage.predictedPerSecond, 35.5);
});
```

Existing `onChunk: chunks.add` tests must be updated in this same task (they will not compile after the API change).

- [ ] **Step 2: Run test to verify it fails**

Run: `flutter test test/acp/agent_connection_test.dart`

Expected: FAIL (`agentThoughtText` / `AgentTurnEvent` undefined, or `onChunk` still the API).

- [ ] **Step 3: Implement forwarding**

In `agent_connection.dart`:

- Add `TurnUsage`, `AgentTurnEvent` hierarchy, `AgentTurnHandler` as in **Interfaces**.
- `agentThoughtText(SessionUpdate)` analog of `agentMessageText`.
- `turnUsageFromUpdate(SessionUpdate)`: if `UsageSessionUpdate`, map `used` → `totalTokens` when meta lacks `totalTokens`; read `_meta` keys with `num` → `int`/`double`.
- `onSessionUpdate`: dispatch thought / message / usage to `onEvent`; ignore other kinds.
- `sendPrompt(..., required AgentTurnHandler onEvent)`.
- Replace `AgentChunkHandler` usages.

Update every `FakeConn` / `_FakeConn` `sendPrompt` to `onEvent`: emit `AgentThoughtDelta` for each `thoughtsToEmit` (default empty), then `AgentMessageDelta` for `chunksToEmit`, then `AgentUsageEvent` if `usageToEmit` is non-null.

- [ ] **Step 4: Run tests to verify they pass**

Run: `flutter test test/acp/agent_connection_test.dart test/chat test/app_shell_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/acp/agent_connection.dart client/test
git commit -m "feat(client): forward ACP thought and usage turn events"
```

---

### Task 5: Chat models + controller accumulation

**Files:**
- Modify: `client/lib/chat/chat_message.dart`
- Modify: `client/lib/catalog/models.dart` (`Agent.providerName`, `ThreadMessage` parts)
- Modify: `client/lib/catalog/catalog_client.dart` only if parsing needs helpers
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/test/chat/chat_controller_test.dart`
- Modify: `client/test/catalog/catalog_client_test.dart` (optional `providerName`)

**Interfaces:**
- Consumes: `AgentTurnEvent`, HTTP message JSON from Task 2
- Produces: `ChatMessage` with thought/usage/model/provider; reload from `ThreadDetail`

- [ ] **Step 1: Write failing controller tests**

```dart
test('send accumulates thought separately from assistant text', () async {
  final conn = FakeConn()
    ..thoughtsToEmit = ['why']
    ..chunksToEmit = ['hello']
    ..usageToEmit = const TurnUsage(predictedPerSecond: 35.5, deltas: 1, stopReason: 'end_turn');
  // connect, create thread, select complete agent, send 'hi'
  expect(c.messages.last.role, ChatRole.assistant);
  expect(c.messages.last.text, 'hello');
  expect(c.messages.last.thought, 'why');
  expect(c.messages.last.model, 'm1');
  expect(c.messages.last.providerName, 'Local'); // agent.providerName
  expect(c.messages.last.usage?.predictedPerSecond, 35.5);
});

test('selectThread maps persisted parts onto ChatMessage', () async {
  // FakeCatalog getThread returns assistant with parts thought/message/usage and model/providerName
  await c.selectThread(id);
  expect(c.messages.last.thought, 'hmm');
  expect(c.messages.last.text, 'hello');
  expect(c.messages.last.providerName, 'Local');
});
```

Extend `Agent.fromJson` test JSON with `'providerName': 'Local'`.

`FakeConn.sendPrompt` should emit thoughts, then message chunks, then usage if set.

On send start, set `model: currentModel` and `providerName` from `agents` matching `selectedAgentId`.

- [ ] **Step 2: Run test to verify it fails**

Run: `flutter test test/chat/chat_controller_test.dart test/catalog/catalog_client_test.dart`

Expected: FAIL (`thought` getter undefined).

- [ ] **Step 3: Implement models and controller**

`ChatMessage`:

```dart
class ChatMessage {
  const ChatMessage({
    required this.role,
    required this.text,
    this.thought,
    this.model,
    this.providerName,
    this.usage,
    this.stopReason,
    this.streamingThought = false,
  });
  final ChatRole role;
  final String text;
  final String? thought;
  final String? model;
  final String? providerName;
  final TurnUsage? usage;
  final String? stopReason;
  final bool streamingThought;
  ChatMessage copyWith({...});
}
```

`ThreadMessage.fromJson`: parse `parts` list; `thought` = first `type==thought` text; `usage` from `type==usage`; `model`/`providerName`/`stopReason` from message fields. Unknown part types ignored.

`ChatController.send`: last assistant starts `text:''`; on `AgentThoughtDelta` append thought and `streamingThought: true`; on `AgentMessageDelta` append text; on `AgentUsageEvent` set usage/stopReason. After `sendPrompt` returns, `streamingThought: false`. `selectThread` maps `ThreadMessage` → `ChatMessage`.

`_refreshSelectedThread` after send may replace messages from GET — keep that so persisted parts win after commit.

- [ ] **Step 4: Run tests to verify they pass**

Run: `flutter test test/chat/chat_controller_test.dart test/catalog/catalog_client_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_message.dart client/lib/chat/chat_controller.dart client/lib/catalog client/test
git commit -m "feat(chat): accumulate thought, usage, and model caption on turns"
```

---

### Task 6: Assistant turn UI

**Files:**
- Create: `client/lib/chat/assistant_turn.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Test: `client/test/chat/chat_screen_test.dart` (and/or `client/test/chat/assistant_turn_test.dart`)

**Interfaces:**
- Consumes: `ChatMessage`, `VisibilityMode` default `collapsed` (inline const until Task 7)
- Produces: caption, Thinking expander, Stats expander

Define `enum VisibilityMode { collapsed, expanded, hidden }` in `assistant_turn.dart`. Defaults: both modes `collapsed`. Task 7 moves the enum to `display_settings.dart`; this file imports it.

- [ ] **Step 1: Write failing widget tests**

```dart
testWidgets('assistant turn shows caption and collapsed thinking', (tester) async {
  await tester.pumpWidget(MaterialApp(
    home: Scaffold(
      body: AssistantTurnTile(
        message: ChatMessage(
          role: ChatRole.assistant,
          text: 'hello',
          thought: 'hmm',
          model: 'm1',
          providerName: 'Local',
          usage: TurnUsage(predictedPerSecond: 35.5, deltas: 1, elapsedMs: 50, stopReason: 'end_turn'),
        ),
      ),
    ),
  ));
  expect(find.text('hello'), findsOneWidget);
  expect(find.textContaining('m1'), findsOneWidget);
  expect(find.textContaining('Local'), findsOneWidget);
  expect(find.textContaining('35.5'), findsOneWidget); // tok/s in caption
  expect(find.text('hmm'), findsNothing); // collapsed
  await tester.tap(find.text('Thinking'));
  await tester.pumpAndSettle();
  expect(find.text('hmm'), findsOneWidget);
});

testWidgets('thinking stays open while streaming then follows collapsed default', (tester) async {
  final conn = FakeConn()
    ..thoughtsToEmit = ['hmm']
    ..chunksToEmit = ['hello']
    ..sendHang = Completer<void>();
  // pump ChatScreen with complete agent + thread, send, do not complete hang yet
  await tester.pump();
  expect(find.text('hmm'), findsOneWidget);
  conn.sendHang!.complete();
  await tester.pumpAndSettle();
  expect(find.text('hmm'), findsNothing);
  expect(find.text('hello'), findsOneWidget);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `flutter test test/chat/chat_screen_test.dart test/chat/assistant_turn_test.dart`

Expected: FAIL (`AssistantTurnTile` not found).

- [ ] **Step 3: Implement widget**

`AssistantTurnTile`:

- Answer `Text(message.text)` (or `…` if empty).
- Caption: `[model, providerName, tok/s].where not empty/null join ' · '`. Format tok/s as `n tok/s` using `predictedPerSecond`.
- Thinking: if `thought` non-empty and mode != hidden: `ExpansionTile` title `Thinking`, initially expanded iff `message.streamingThought || mode == expanded`.
- Stats: if mode != hidden: `ExpansionTile` title `Stats`, initially expanded iff `mode == expanded`. Body lines for each present usage field + `stopReason`.
- User messages stay the existing bubble in `chat_screen.dart`; assistant rows use `AssistantTurnTile`.

Use a new `ValueKey` on `ExpansionTile` including `streamingThought` so it rebuilds initial state at turn end (`ExpansionTile` does not update `initiallyExpanded` otherwise). Example: `Key('thinking-${message.streamingThought}-${message.thought.hashCode}')`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `flutter test test/chat/`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat client/test/chat
git commit -m "feat(chat): show thinking and stats in collapsible assistant turns"
```

---

### Task 7: Settings Chat tab + persistence

**Files:**
- Create: `client/lib/chat/display_settings.dart`
- Create: `client/lib/settings/chat_tab.dart`
- Modify: `client/lib/settings/settings_page.dart`
- Modify: `client/lib/app_shell.dart` (pass settings into Chat + Settings)
- Modify: `client/lib/main.dart` if the app must `await DisplaySettings.load()`
- Modify: `client/pubspec.yaml` (`shared_preferences`)
- Modify: `client/lib/chat/chat_screen.dart` (read settings)
- Test: `client/test/settings/chat_tab_test.dart`
- Modify: `README.md` (one short paragraph)

**Interfaces:**
- Consumes: `VisibilityMode`, `AssistantTurnTile`
- Produces: persisted defaults; Settings → Chat tab

- [ ] **Step 1: Add dependency and failing tests**

```bash
cd client && flutter pub add shared_preferences
```

```dart
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
  });

  test('defaults are collapsed', () async {
    final s = await ChatDisplaySettings.load();
    expect(s.thinking, VisibilityMode.collapsed);
    expect(s.stats, VisibilityMode.collapsed);
  });

  test('setThinking persists', () async {
    final s = await ChatDisplaySettings.load();
    await s.setThinking(VisibilityMode.hidden);
    final s2 = await ChatDisplaySettings.load();
    expect(s2.thinking, VisibilityMode.hidden);
  });

  testWidgets('Chat tab updates thinking visibility', (tester) async {
    final s = await ChatDisplaySettings.load();
    await tester.pumpWidget(MaterialApp(
      home: ChatTab(settings: s),
    ));
    expect(find.text('Thinking'), findsOneWidget);
    await tester.tap(find.byKey(const Key('thinking-visibility')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Hidden').last);
    await tester.pumpAndSettle();
    expect(s.thinking, VisibilityMode.hidden);
  });
}
```

Also a widget test: `AssistantTurnTile` with `thinkingMode: hidden` does not show `Thinking` but still shows the answer and caption.

- [ ] **Step 2: Run test to verify it fails**

Run: `flutter test test/settings/chat_tab_test.dart`

Expected: FAIL (`ChatDisplaySettings` / `ChatTab` missing).

- [ ] **Step 3: Implement settings**

Keys: `chat.thinkingVisibility`, `chat.statsVisibility` (values `collapsed|expanded|hidden`).

`ChatDisplaySettings extends ChangeNotifier` with `load`, `setThinking`, `setStats`.

`ChatTab`: two `DropdownButtonFormField<VisibilityMode>` labeled `Thinking` and `Stats`.

`SettingsPage`: `length: 3`, tabs Providers / Agents / Chat. Constructor takes `ChatDisplaySettings`.

`AppShell`: create/load settings once (or load in `main` and pass down). `ChatScreen` listens and passes modes into `AssistantTurnTile`. Caption is **not** hidden when Stats is hidden.

`README.md`: note that thinking models stream a Thinking expander (collapsed after the turn) and that Stats/tok/s persist on the thread.

- [ ] **Step 4: Run tests to verify they pass**

Run: `flutter test` and `go -C controlplane test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client README.md
git commit -m "feat(settings): chat visibility defaults for thinking and stats"
```

---

## Self-review

**Spec coverage**

| Spec item | Task |
| --- | --- |
| `reasoning_content` → thought chunks | 1, 3 |
| `include_usage` + timings + TTFT/elapsed/deltas | 1, 3 |
| Thinking without content is an error, no commit | 1, 3 (empty assistant) |
| `parts` jsonb + model/provider/stopReason persist | 2, 3 |
| LLM history is visible text only | 3 |
| `providerName` on agents for live caption | 2, 5 |
| ACP thought/usage forwarding | 4 |
| Caption model · provider · tok/s | 5, 6 |
| Thinking opens while streaming, collapses after | 6 |
| Stats expander; always-on caption | 6, 7 |
| Settings collapsed/expanded/hidden | 7 |
| Unknown part types skipped | 5 `fromJson` |
| `max_tokens` / `refusal` still commit | 3 |
| CLI unbound: stream, no thread write | 3 (existing unbound path) |
| No `sent` / tools / per-chat override | omitted by YAGNI |

**Type consistency:** `StreamEvent` / `Usage` (Go) → ACP `_meta` → `TurnUsage` (Dart) → `MessagePart` JSON field names in the spec (`ttftMs`, `predictedPerSecond`, …). `CommitTurn(..., AssistantTurn)` is the only persist API after Task 2. `sendPrompt(..., onEvent:)` is the only client stream API after Task 4.

**Placeholders:** none. If Go `SessionUsageUpdate.SessionUpdate` discriminator is required (it is — set `"usage_update"`). If `acpd` `UsageSessionUpdate.meta` values are `Object?`, parse with `num`.
