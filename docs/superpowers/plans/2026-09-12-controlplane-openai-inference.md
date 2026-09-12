# Control Plane OpenAI Inference Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the live echo provider with OpenAI-compatible Chat Completions streaming so Flutter/CLI prompts go through Unsloth Studio and stream back as ACP `session/update`s, with full in-memory session history.

**Architecture:** Env-loaded OpenAI config at process start; a `ChatStreamer` interface with an HTTP SSE OpenAI client; `runtime.Session` holds the transcript; `Agent.Prompt` appends the user turn, streams deltas as ACP updates, then appends the assistant turn. Cancel aborts the request context and skips committing a partial assistant message.

**Tech Stack:** Go 1.22+, `github.com/coder/acp-go-sdk@v0.13.5`, stdlib `net/http` + SSE over Chat Completions, existing WebSocket ACP path. No new third-party LLM SDKs.

## Global Constraints

- Module root: `controlplane/` (`github.com/tryy3/agent-fabric`); run tests with `go -C controlplane test ./...`.
- Required env (live binary only): `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL` — fail before listen if any missing.
- Streaming only (`stream: true`); forward each content delta as `acp.UpdateAgentMessageText`.
- Full session transcript in memory until disconnect; no system prompt in this slice.
- No echo fallback when env is unset.
- CI/tests must not call real Unsloth; use fakes / `httptest`.
- No Flutter code changes; README documents env + Unsloth smoke.
- Follow TDD: failing test → implement → pass → commit per task.
- Prefer small packages under `controlplane/internal/`; keep ACP agent thin.

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/config/config.go` | Load/validate OpenAI env; normalize base URL |
| `controlplane/internal/config/config_test.go` | Env success/failure cases |
| `controlplane/internal/runtime/session.go` | Session + transcript append/get helpers |
| `controlplane/internal/runtime/session_test.go` | Transcript append/order tests |
| `controlplane/internal/provider/message.go` | `Message` + `ChatStreamer` interface; keep `PromptText` |
| `controlplane/internal/provider/openai.go` | OpenAI Chat Completions SSE client |
| `controlplane/internal/provider/openai_test.go` | `httptest` streaming / error / empty tests |
| `controlplane/internal/provider/echo.go` | Delete (or strip `Echo`; `PromptText` moves to `message.go` / `prompt.go`) |
| `controlplane/internal/provider/echo_test.go` | Drop `TestEchoReturnsInput`; keep PromptText tests |
| `controlplane/internal/agent/agent.go` | Inject streamer; Prompt/Cancel with history + cancel map |
| `controlplane/internal/agent/agent_test.go` | Fake streamer multi-chunk + multi-turn + cancel |
| `controlplane/internal/transport/ws/handler.go` | Pass streamer into `agent.New` |
| `controlplane/internal/server/server.go` | Thread streamer into mux/handler |
| `controlplane/internal/server/server_test.go` | Fake streamer WS turn (replace echo assertion) |
| `controlplane/cmd/controlplane/main.go` | Load config; construct OpenAI client; wire server |
| `README.md` | Env vars + Unsloth smoke; remove “echo agent” wording for live path |

---

### Task 1: OpenAI env config

**Files:**
- Create: `controlplane/internal/config/config.go`
- Create: `controlplane/internal/config/config_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  - `config.Config` with fields `BaseURL`, `APIKey`, `Model string`
  - `config.Load() (Config, error)` — reads `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`; trims trailing `/` on BaseURL; returns error naming the first missing variable if any is empty

- [ ] **Step 1: Write the failing test**

Create `controlplane/internal/config/config_test.go`:

```go
package config_test

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/config"
)

func TestLoadSuccessTrimsTrailingSlash(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1/")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "my-model")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:8000/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
	if cfg.Model != "my-model" {
		t.Fatalf("Model = %q", cfg.Model)
	}
}

func TestLoadMissingBaseURL(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "my-model")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_BASE_URL") {
		t.Fatalf("error = %v, want OPENAI_BASE_URL", err)
	}
}

func TestLoadMissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "my-model")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("error = %v, want OPENAI_API_KEY", err)
	}
}

func TestLoadMissingModel(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_MODEL") {
		t.Fatalf("error = %v, want OPENAI_MODEL", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/config/ -v`

Expected: FAIL (package or `config.Load` undefined).

- [ ] **Step 3: Write minimal implementation**

Create `controlplane/internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func Load() (Config, error) {
	cfg := Config{
		BaseURL: strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/"),
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
	}
	switch {
	case cfg.BaseURL == "":
		return Config{}, fmt.Errorf("missing required env OPENAI_BASE_URL")
	case cfg.APIKey == "":
		return Config{}, fmt.Errorf("missing required env OPENAI_API_KEY")
	case cfg.Model == "":
		return Config{}, fmt.Errorf("missing required env OPENAI_MODEL")
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/config/ -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/config/config.go controlplane/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
Add OpenAI env config loader for the control plane.

EOF
)"
```

---

### Task 2: Session transcript in runtime

**Files:**
- Modify: `controlplane/internal/runtime/session.go`
- Modify: `controlplane/internal/runtime/session_test.go`
- Modify: `controlplane/internal/provider/echo.go` (move `PromptText` only if needed later; leave for Task 3)

**Interfaces:**
- Consumes: existing `Store`, `Session`, `Create`/`Get`/`Delete`
- Produces:
  - `runtime.Message` with `Role string`, `Content string` (`"user"` / `"assistant"`)
  - `Session.Messages []Message` (zero value empty slice on Create)
  - `(*Store).Append(id string, msg Message) error` — append under lock; error if session missing
  - `(*Store).Messages(id string) ([]Message, bool)` — copy of transcript (or document that callers must not mutate; prefer returning a copy)

- [ ] **Step 1: Write the failing transcript tests**

Append to `controlplane/internal/runtime/session_test.go`:

```go
func TestAppendBuildsTranscript(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(id, runtime.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := store.Append(id, runtime.Message{Role: "assistant", Content: "hello"}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}
	msgs, ok := store.Messages(id)
	if !ok {
		t.Fatal("session missing")
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Fatalf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hello" {
		t.Fatalf("msgs[1] = %+v", msgs[1])
	}
}

func TestAppendUnknownSession(t *testing.T) {
	store := runtime.NewStore()
	err := store.Append("missing", runtime.Message{Role: "user", Content: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMessagesReturnsCopy(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(id, runtime.Message{Role: "user", Content: "a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	msgs, ok := store.Messages(id)
	if !ok {
		t.Fatal("missing")
	}
	msgs[0].Content = "mutated"
	again, _ := store.Messages(id)
	if again[0].Content != "a" {
		t.Fatalf("store mutated via returned slice: %q", again[0].Content)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/runtime/ -v -run 'Append|Messages'`

Expected: FAIL (`Append` / `Message` undefined).

- [ ] **Step 3: Write minimal implementation**

Update `controlplane/internal/runtime/session.go` so `Session` and store methods look like:

```go
type Message struct {
	Role    string
	Content string
}

type Session struct {
	ID         string
	Definition Definition
	Messages   []Message
}

// Create unchanged except Session includes Messages: nil / empty.

func (s *Store) Append(id string, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	sess.Messages = append(sess.Messages, msg)
	s.sessions[id] = sess
	return nil
}

func (s *Store) Messages(id string) ([]Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	out := make([]Message, len(sess.Messages))
	copy(out, sess.Messages)
	return out, true
}
```

Keep existing `Get` returning the full `Session` (including `Messages`). Prefer `Append`/`Messages` for mutation so concurrent prompts do not race on a copied struct.

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/runtime/ -v`

Expected: PASS (including existing create/delete tests).

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/runtime/session.go controlplane/internal/runtime/session_test.go
git commit -m "$(cat <<'EOF'
Store in-memory chat transcript on ACP sessions.

EOF
)"
```

---

### Task 3: OpenAI Chat Completions streamer

**Files:**
- Create: `controlplane/internal/provider/streamer.go` (`Message`, `ChatStreamer`, keep `PromptText` here or in `prompt.go`)
- Create: `controlplane/internal/provider/openai.go`
- Create: `controlplane/internal/provider/openai_test.go`
- Modify: `controlplane/internal/provider/echo.go` — delete `Echo` (and file if empty)
- Modify: `controlplane/internal/provider/echo_test.go` — remove `TestEchoReturnsInput`; keep PromptText test (rename file to `prompt_test.go` if you move `PromptText`)

**Interfaces:**
- Consumes: base URL / API key / model (+ optional `*http.Client`) when constructing the client
- Produces:
  - `provider.ChatStreamer` with `StreamChat(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error`
  - `provider.NewOpenAI(baseURL, apiKey, model string, httpClient *http.Client) *OpenAI` (`httpClient` nil → `http.DefaultClient`)
  - `runtime.Message` JSON tags `json:"role"` / `json:"content"` (add in this task if missing)
  - Behavior:
    - POST `{baseURL}/chat/completions` with JSON `model`, `stream: true`, `messages: [{role, content}, ...]`
    - Header `Authorization: Bearer {apiKey}`, `Content-Type: application/json`, `Accept: text/event-stream`
    - Parse SSE lines `data: ...`; ignore `[DONE]`; for each JSON chunk read `choices[0].delta.content` if present and non-empty → `onDelta`
    - Non-2xx → error including status and body snippet
    - Context cancel → return `ctx.Err()` (or wrapped)
    - If stream completes with zero content deltas → return error `empty assistant response` (do not succeed silently)
  - Keep `provider.PromptText(blocks []acp.ContentBlock) string`

- [ ] **Step 1: Write the failing OpenAI streamer tests**

Create `controlplane/internal/provider/openai_test.go`:

```go
package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestOpenAIStreamsDeltas(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk-test", "m", srv.Client())
	var parts []string
	err := client.StreamChat(context.Background(), []runtime.Message{
		{Role: "user", Content: "hi"},
	}, func(delta string) error {
		parts = append(parts, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if strings.Join(parts, "") != "Hello" {
		t.Fatalf("parts = %#v", parts)
	}
	if gotBody["model"] != "m" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	if gotBody["stream"] != true {
		t.Fatalf("stream = %v", gotBody["stream"])
	}
}

func TestOpenAIHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk", "m", srv.Client())
	err := client.StreamChat(context.Background(), []runtime.Message{{Role: "user", Content: "x"}}, func(string) error { return nil })
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenAIEmptyAssistant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk", "m", srv.Client())
	err := client.StreamChat(context.Background(), []runtime.Message{{Role: "user", Content: "x"}}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want empty", err)
	}
}

func TestOpenAICancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := provider.NewOpenAI(srv.URL+"/v1", "sk", "m", srv.Client())
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.StreamChat(ctx, []runtime.Message{{Role: "user", Content: "x"}}, func(string) error { return nil })
	}()
	<-started
	cancel()
	err := <-errCh
	if err == nil {
		t.Fatal("expected cancel error")
	}
}
```

Also add a compile-time check in `streamer.go` later: `var _ ChatStreamer = (*OpenAI)(nil)`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/provider/ -v -run OpenAI`

Expected: FAIL (`NewOpenAI` / `StreamChat` undefined).

- [ ] **Step 3: Write minimal implementation**

Create `controlplane/internal/provider/streamer.go`:

```go
package provider

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type ChatStreamer interface {
	StreamChat(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error
}

func PromptText(blocks []acp.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Text != nil {
			out += b.Text.Text
		}
	}
	return out
}
```

Create `controlplane/internal/provider/openai.go` implementing SSE parsing roughly as:

```go
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/runtime"
)

type OpenAI struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAI(baseURL, apiKey, model string, httpClient *http.Client) *OpenAI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAI{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: httpClient,
	}
}

func (o *OpenAI) StreamChat(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error {
	// build request body with role/content + stream:true + model
	// POST o.baseURL + "/chat/completions"
	// read SSE with bufio.Scanner; for lines with prefix "data: ", trim and handle [DONE] / JSON
	// track gotContent; if !gotContent at end return fmt.Errorf("empty assistant response")
}
```

Implement JSON shapes:

```go
type chatRequest struct {
	Model    string           `json:"model"`
	Stream   bool             `json:"stream"`
	Messages []runtime.Message `json:"messages"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}
```

Ensure `runtime.Message` JSON tags are `json:"role"` and `json:"content"` — add tags on `runtime.Message` in this task if not already present:

```go
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
```

Delete `Echo` from `echo.go`; if the file only had `Echo` + `PromptText`, delete `echo.go` after moving `PromptText` to `streamer.go`. Update `echo_test.go` → keep only PromptText test (rename to `prompt_test.go` optional).

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/provider/ -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/provider/ controlplane/internal/runtime/session.go
git commit -m "$(cat <<'EOF'
Add OpenAI Chat Completions SSE streamer.

EOF
)"
```

---

### Task 4: Agent Prompt/Cancel with history and streamer

**Files:**
- Modify: `controlplane/internal/agent/agent.go`
- Modify: `controlplane/internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `provider.ChatStreamer`, `provider.PromptText`, `runtime.Store.Append` / `Messages`
- Produces:
  - `agent.New(store *runtime.Store, streamer provider.ChatStreamer) *Agent`
  - `Prompt`:
    1. Lookup session; error if missing / conn nil / streamer nil
    2. `text := provider.PromptText(params.Prompt)`
    3. `store.Append(sid, runtime.Message{Role:"user", Content:text})`
    4. Register cancel for `sid` (context derived from `ctx`); clear on return
    5. `msgs, _ := store.Messages(sid)`
    6. `var full strings.Builder`; `streamer.StreamChat(promptCtx, msgs, func(delta string) error { full.WriteString(delta); return conn.SessionUpdate(..., UpdateAgentMessageText(delta)) })`
    7. On streamer error: return error (user turn remains; no assistant append)
    8. On success: if `full.Len()==0` should already be streamer error; else `Append` assistant with `full.String()`; return `end_turn`
  - `Cancel`: if in-flight cancel for `params.SessionId`, call it; return nil
  - Concurrent map: `cancels map[string]context.CancelFunc` protected by `a.mu`

- [ ] **Step 1: Write failing agent tests with a fake streamer**

Replace/extend `controlplane/internal/agent/agent_test.go`. Keep the capture client helpers. Add:

```go
type fakeStreamer struct {
	mu           sync.Mutex
	deltas       []string
	err          error
	lastMessages []runtime.Message
	blockUntil   <-chan struct{} // optional: hold stream open for cancel test
}

func (f *fakeStreamer) StreamChat(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error {
	f.mu.Lock()
	f.lastMessages = append([]runtime.Message(nil), messages...)
	deltas := append([]string(nil), f.deltas...)
	blockUntil := f.blockUntil
	err := f.err
	f.mu.Unlock()

	if blockUntil != nil {
		select {
		case <-blockUntil:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for _, d := range deltas {
		if err := onDelta(d); err != nil {
			return err
		}
	}
	return err
}
```

Rewrite `TestEchoTurnOverPipes` into `TestStreamedTurnOverPipes`:

- `ag := agent.New(store, &fakeStreamer{deltas: []string{"Hel", "lo"}})`
- Assert joined chunks == `"Hello"`

Add `TestMultiTurnSendsHistory`:

- First prompt `"hi"` with deltas `[]string{"yo"}`
- Second prompt `"again"` with deltas `[]string{"ok"}`
- After second `StreamChat`, assert `lastMessages` equals:

```text
[{user hi} {assistant yo} {user again}]
```

Add `TestCancelAbortsInFlightPrompt`:

- `block := make(chan struct{})` — never close during test (or close after assert)
- Start Prompt in a goroutine with fake that waits on `blockUntil` or `ctx.Done()`
- Call `csc` cancel notification / `ag.Cancel` with the session id (use the agent’s Cancel via client if ACP exposes it; otherwise call `ag.Cancel` directly after starting Prompt)
- Assert Prompt returns an error / context canceled
- Assert `store.Messages` has only the user message (no assistant)

Practical cancel test without full ACP cancel RPC:

```go
func TestCancelAbortsInFlightPrompt(t *testing.T) {
	store := runtime.NewStore()
	started := make(chan struct{})
	fs := &fakeStreamer{
		streamFn: func(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	// Prefer implementing fakeStreamer.StreamChat with an optional streamFn override field.
	ag := agent.New(store, fs)
	// set up pipes + Initialize + NewSession as in existing test
	// then:
	go func() {
		<-started
		_ = ag.Cancel(context.Background(), acp.CancelNotification{SessionId: sess.SessionId})
	}()
	_, err := csc.Prompt(ctx, acp.PromptRequest{SessionId: sess.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("hi")}})
	if err == nil {
		t.Fatal("expected prompt error after cancel")
	}
	msgs, _ := store.Messages(string(sess.SessionId))
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("history = %+v, want only user", msgs)
	}
}
```

If `fakeStreamer` uses `streamFn`, define:

```go
type fakeStreamer struct {
	mu           sync.Mutex
	deltas       []string
	err          error
	lastMessages []runtime.Message
	streamFn     func(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error
}

func (f *fakeStreamer) StreamChat(ctx context.Context, messages []runtime.Message, onDelta func(string) error) error {
	f.mu.Lock()
	f.lastMessages = append([]runtime.Message(nil), messages...)
	fn := f.streamFn
	deltas := append([]string(nil), f.deltas...)
	err := f.err
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, messages, onDelta)
	}
	for _, d := range deltas {
		if e := onDelta(d); e != nil {
			return e
		}
	}
	return err
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/agent/ -v`

Expected: FAIL (`agent.New` wrong arity / echo behavior).

- [ ] **Step 3: Write minimal implementation**

Update `controlplane/internal/agent/agent.go`:

```go
type Agent struct {
	store    *runtime.Store
	streamer provider.ChatStreamer

	mu       sync.Mutex
	conn     *acp.AgentSideConnection
	sessions map[string]struct{}
	cancels  map[string]context.CancelFunc
	closed   bool
}

func New(store *runtime.Store, streamer provider.ChatStreamer) *Agent {
	return &Agent{
		store:    store,
		streamer: streamer,
		sessions: make(map[string]struct{}),
		cancels:  make(map[string]context.CancelFunc),
	}
}

func (a *Agent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	sid := string(params.SessionId)
	if _, ok := a.store.Get(sid); !ok {
		return acp.PromptResponse{}, fmt.Errorf("session %s not found", sid)
	}
	conn := a.connection()
	if conn == nil {
		return acp.PromptResponse{}, fmt.Errorf("agent connection not set")
	}
	if a.streamer == nil {
		return acp.PromptResponse{}, fmt.Errorf("streamer not configured")
	}

	text := provider.PromptText(params.Prompt)
	if err := a.store.Append(sid, runtime.Message{Role: "user", Content: text}); err != nil {
		return acp.PromptResponse{}, err
	}

	promptCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	if prev, ok := a.cancels[sid]; ok {
		prev()
	}
	a.cancels[sid] = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		delete(a.cancels, sid)
		a.mu.Unlock()
	}()

	msgs, ok := a.store.Messages(sid)
	if !ok {
		return acp.PromptResponse{}, fmt.Errorf("session %s not found", sid)
	}

	var full strings.Builder
	err := a.streamer.StreamChat(promptCtx, msgs, func(delta string) error {
		full.WriteString(delta)
		return conn.SessionUpdate(promptCtx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update:    acp.UpdateAgentMessageText(delta),
		})
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	if err := a.store.Append(sid, runtime.Message{Role: "assistant", Content: full.String()}); err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *Agent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	sid := string(params.SessionId)
	a.mu.Lock()
	cancel := a.cancels[sid]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/agent/ -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent/
git commit -m "$(cat <<'EOF'
Wire ACP Prompt/Cancel to ChatStreamer and session history.

EOF
)"
```

---

### Task 5: Wire server, main, and WS integration test

**Files:**
- Modify: `controlplane/internal/transport/ws/handler.go`
- Modify: `controlplane/internal/server/server.go`
- Modify: `controlplane/internal/server/server_test.go`
- Modify: `controlplane/cmd/controlplane/main.go`

**Interfaces:**
- Consumes: `config.Load`, `provider.NewOpenAI`, `agent.New(store, streamer)`
- Produces:
  - `ws.Handler(store *runtime.Store, streamer provider.ChatStreamer) http.Handler`
  - `server.NewMux(store, streamer)` / `server.New(addr, store, streamer)`
  - `main`: `cfg, err := config.Load(); if err != nil { log.Fatal(err) }`; `streamer := provider.NewOpenAI(cfg.BaseURL, cfg.APIKey, cfg.Model, nil)`; pass into `server.New`

- [ ] **Step 1: Update WS integration test to use a fake streamer**

In `controlplane/internal/server/server_test.go`, define a small fake (same shape as agent test) that streams `[]string{"hel", "lo"}` for any prompt, then:

```go
store := runtime.NewStore()
streamer := &fakeStreamer{deltas: []string{"hel", "lo"}}
srv := httptest.NewServer(server.NewMux(store, streamer))
```

Rename test to `TestWebSocketStreamedTurn` and assert joined chunks == `"hello"`.

Optionally add a second-prompt history assertion by making the fake record `lastMessages` and checking length ≥ 3 after two prompts — preferred if cheap:

```go
// after first prompt completes with deltas "a"
// second prompt "b" with deltas "c"
streamer.mu.Lock()
n := len(streamer.lastMessages)
streamer.mu.Unlock()
if n != 3 { ... } // user, assistant, user
```

For that, the fake must be shared and mutable across prompts (reset deltas between prompts or always return fixed deltas).

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/server/ -v`

Expected: FAIL (arity / compile errors on `NewMux`).

- [ ] **Step 3: Wire handler, server, and main**

`handler.go`:

```go
func Handler(store *runtime.Store, streamer provider.ChatStreamer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ... upgrade ...
		ag := agent.New(store, streamer)
		// rest unchanged
	})
}
```

`server.go`:

```go
func NewMux(store *runtime.Store, streamer provider.ChatStreamer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, streamer))
	return mux
}

func New(addr string, store *runtime.Store, streamer provider.ChatStreamer) *http.Server {
	return &http.Server{Addr: addr, Handler: NewMux(store, streamer)}
}
```

`main.go`:

```go
package main

import (
	"flag"
	"log"
	"log/slog"

	"github.com/tryy3/agent-fabric/internal/config"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	store := runtime.NewStore()
	streamer := provider.NewOpenAI(cfg.BaseURL, cfg.APIKey, cfg.Model, nil)
	srv := server.New(*addr, store, streamer)
	slog.Info("controlplane listening", "addr", *addr, "acp", "/acp", "model", cfg.Model)
	log.Fatal(srv.ListenAndServe())
}
```

- [ ] **Step 4: Run all controlplane tests**

Run: `go -C controlplane test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/transport/ws/handler.go controlplane/internal/server/ controlplane/cmd/controlplane/main.go
git commit -m "$(cat <<'EOF'
Wire OpenAI streamer through controlplane server startup.

EOF
)"
```

---

### Task 6: README Unsloth smoke docs

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: env names and run commands from the spec
- Produces: documented live path (no code API)

- [ ] **Step 1: Update README run sections**

Replace echo-centric wording with OpenAI/Unsloth requirements. Ensure these appear:

```bash
export OPENAI_BASE_URL=http://127.0.0.1:<unsloth-port>/v1
export OPENAI_API_KEY=sk-local
export OPENAI_MODEL=<model-id>

go -C controlplane run ./cmd/controlplane
```

Note: process exits if env vars are missing.

Update Flutter section: “Send a message; the agent streams a model reply (requires Unsloth/OpenAI-compatible endpoint).”

Keep: `go -C controlplane test ./...` is offline (fakes).

Link design: `docs/superpowers/specs/2026-09-12-controlplane-openai-inference-design.md`.

- [ ] **Step 2: Sanity-check docs against code**

Confirm README env names match `config.Load` exactly (`OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`).

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
Document OpenAI env and Unsloth smoke for the control plane.

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Env-only config; fail fast | Task 1, Task 5 `main` |
| Full in-memory session history | Task 2, Task 4 |
| OpenAI Chat Completions `stream: true` | Task 3 |
| ACP `session/update` deltas | Task 4 |
| Cancel → abort HTTP / no partial assistant | Task 3 cancel + Task 4 Cancel |
| Empty assistant → error, no append | Task 3 |
| Provider error keeps user turn, no assistant | Task 4 |
| Offline tests with fakes | Tasks 3–5 |
| README Unsloth smoke | Task 6 |
| No Flutter changes | (none) |
| No echo fallback | Task 5 `main` always OpenAI |

## Plan self-review notes

- Types aligned: `runtime.Message` is the transcript + Chat Completions message shape; `provider.ChatStreamer` takes `[]runtime.Message`.
- `agent.New(store, streamer)` arity updated everywhere (handler, tests).
- No TBD placeholders; fake streamer defined for agent/server tests.
