# Control Plane ACP Echo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Go control plane that speaks ACP v1 over WebSocket `/acp` with a hardcoded echo agent, proven by integration tests and a small CLI.

**Architecture:** `coder/acp-go-sdk` owns JSON-RPC (newline-delimited). A thin WebSocket adapter maps each WS text frame ↔ one NDJSON line so the same Agent works over WS. One in-memory runtime pins a hardcoded `echo` definition per session; prompts stream the user text back via `session/update`.

**Tech Stack:** Go 1.22+, `github.com/coder/acp-go-sdk@v0.13.5`, `github.com/gorilla/websocket`, Nix flake `devShell` with `go` + `gopls`, stdlib `net/http` + `httptest`.

## Global Constraints

- ACP v1 only; no catalog HTTP API, auth, memory, MCP, Docker, Flutter, or real LLM in this plan.
- Scripted echo only — no network calls in provider or CI tests.
- WebSocket endpoint path is exactly `/acp`.
- Default listen address `:8080`.
- Module path: `github.com/tryy3/agent-fabric`.
- Pin SDK: `github.com/coder/acp-go-sdk@v0.13.5`.
- Prefer small packages under `internal/`; no premature abstractions beyond the file map below.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `go.mod` / `go.sum` | Module root and deps |
| `flake.nix` | Add `go`, `gopls` to `devShell` |
| `internal/runtime/definition.go` | Hardcoded `echo` definition snapshot type |
| `internal/runtime/session.go` | In-memory session map + pin definition |
| `internal/runtime/session_test.go` | Session create/lookup/close tests |
| `internal/provider/echo.go` | Extract prompt text; produce echo string |
| `internal/provider/echo_test.go` | Echo text extraction / formatting tests |
| `internal/agent/agent.go` | `acp.Agent` impl: initialize, session lifecycle, prompt → SessionUpdate |
| `internal/agent/stubs.go` | Remaining `Agent` methods → `MethodNotFound` / no-ops |
| `internal/agent/agent_test.go` | Pipe-based ACP client↔agent turn (no WS) |
| `internal/transport/ws/bridge.go` | Gorilla WS ↔ `io.Reader`/`io.Writer` NDJSON bridge |
| `internal/transport/ws/bridge_test.go` | Frame ↔ line framing tests |
| `internal/transport/ws/handler.go` | HTTP upgrade handler; wires SDK agent connection |
| `internal/server/server.go` | `http.Server` mux: `/acp` |
| `internal/server/server_test.go` | httptest WebSocket integration test |
| `cmd/controlplane/main.go` | Entrypoint: listen and serve |
| `cmd/acp-cli/main.go` | Manual WS client: init → new session → prompt → print |
| `README.md` | How to run server + CLI + tests |

---

### Task 1: Go module and Nix tooling

**Files:**
- Create: `go.mod`
- Modify: `flake.nix`
- Modify: `.gitignore` (add common Go build artifacts if missing)
- Test: verify via shell commands (no unit test file)

**Interfaces:**
- Consumes: none
- Produces: module `github.com/tryy3/agent-fabric` that can `go test ./...` (empty packages OK)

- [ ] **Step 1: Initialize the Go module**

```bash
cd /home/tryy3/src/agent-fabric
go mod init github.com/tryy3/agent-fabric
```

Expected: creates `go.mod` with `module github.com/tryy3/agent-fabric`.

- [ ] **Step 2: Add ACP SDK and WebSocket dependency**

```bash
go get github.com/coder/acp-go-sdk@v0.13.5
go get github.com/gorilla/websocket@v1.5.3
go mod tidy
```

Expected: `go.mod` / `go.sum` list both modules; no errors.

- [ ] **Step 3: Update Nix flake packages**

Replace the empty `packages` list in `flake.nix` with:

```nix
packages = with pkgs; [
  go
  gopls
];
```

- [ ] **Step 4: Verify tooling**

```bash
# reload direnv / enter nix shell if needed, then:
go version
go list -m github.com/coder/acp-go-sdk
```

Expected: Go version printed; module path resolves to `v0.13.5`.

- [ ] **Step 5: Ignore Go binaries**

Append to `.gitignore` if not already present:

```gitignore
# Go
bin/
*.exe
*.test
coverage.out
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum flake.nix .gitignore
git commit -m "$(cat <<'EOF'
Add Go module and Nix toolchain for the control plane.

EOF
)"
```

---

### Task 2: Runtime sessions and echo provider

**Files:**
- Create: `internal/runtime/definition.go`
- Create: `internal/runtime/session.go`
- Create: `internal/runtime/session_test.go`
- Create: `internal/provider/echo.go`
- Create: `internal/provider/echo_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  - `runtime.Definition` with fields `ID`, `Name`, `Version string`
  - `runtime.EchoDefinition() Definition` — returns hardcoded `{ID:"echo", Name:"Echo", Version:"1"}`
  - `runtime.Store` with:
    - `NewStore() *Store`
    - `Create(def Definition) (sessionID string, err error)`
    - `Get(sessionID string) (Session, bool)`
    - `Delete(sessionID string)`
  - `runtime.Session` with `ID string`, `Definition Definition`
  - `provider.PromptText(blocks []acp.ContentBlock) string` — concatenates text blocks
  - `provider.Echo(text string) string` — returns `text` unchanged (identity echo)

- [ ] **Step 1: Write failing runtime session tests**

Create `internal/runtime/session_test.go`:

```go
package runtime_test

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestCreatePinsEchoDefinition(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty session id")
	}
	sess, ok := store.Get(id)
	if !ok {
		t.Fatal("session not found")
	}
	if sess.Definition.ID != "echo" {
		t.Fatalf("pinned id = %q, want echo", sess.Definition.ID)
	}
	if sess.Definition.Version != "1" {
		t.Fatalf("version = %q, want 1", sess.Definition.Version)
	}
}

func TestDeleteRemovesSession(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.Delete(id)
	if _, ok := store.Get(id); ok {
		t.Fatal("expected session gone after Delete")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/runtime/ -v
```

Expected: FAIL — packages/types undefined.

- [ ] **Step 3: Implement runtime**

Create `internal/runtime/definition.go`:

```go
package runtime

type Definition struct {
	ID      string
	Name    string
	Version string
}

func EchoDefinition() Definition {
	return Definition{ID: "echo", Name: "Echo", Version: "1"}
}
```

Create `internal/runtime/session.go`:

```go
package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type Session struct {
	ID         string
	Definition Definition
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

func (s *Store) Create(def Definition) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = Session{ID: id, Definition: def}
	return id, nil
}

func (s *Store) Get(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func newID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	return "sess_" + hex.EncodeToString(b[:]), nil
}
```

- [ ] **Step 4: Run runtime tests**

```bash
go test ./internal/runtime/ -v
```

Expected: PASS.

- [ ] **Step 5: Write failing provider tests**

Create `internal/provider/echo_test.go`:

```go
package provider_test

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestPromptTextConcatenatesTextBlocks(t *testing.T) {
	got := provider.PromptText([]acp.ContentBlock{
		acp.TextBlock("hello"),
		acp.TextBlock(" "),
		acp.TextBlock("world"),
	})
	if got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestEchoReturnsInput(t *testing.T) {
	if got := provider.Echo("ping"); got != "ping" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 6: Run provider tests to verify they fail**

```bash
go test ./internal/provider/ -v
```

Expected: FAIL — undefined symbols.

- [ ] **Step 7: Implement provider**

Create `internal/provider/echo.go`:

```go
package provider

import acp "github.com/coder/acp-go-sdk"

func PromptText(blocks []acp.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Text != nil {
			out += b.Text.Text
		}
	}
	return out
}

func Echo(text string) string {
	return text
}
```

Note: if `ContentBlock` field names differ in v0.13.5, adjust to match the SDK (inspect `acp.TextBlock` return type / struct tags). Prefer compiling against the SDK rather than guessing alternate field names.

- [ ] **Step 8: Run provider tests**

```bash
go test ./internal/provider/ -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/runtime internal/provider
git commit -m "$(cat <<'EOF'
Add in-memory session store and echo provider.

EOF
)"
```

---

### Task 3: ACP Agent over stdio pipes

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/stubs.go`
- Create: `internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `runtime.Store`, `runtime.EchoDefinition`, `provider.PromptText`, `provider.Echo`
- Produces:
  - `agent.New(store *runtime.Store) *Agent`
  - `(*Agent) SetAgentConnection(conn *acp.AgentSideConnection)` (for `acp.AgentConnAware` if required by SDK pattern)
  - Full `acp.Agent` implementation; unused methods return `acp.NewMethodNotFound(...)` like the SDK example
  - On `Prompt`: look up session; stream `acp.UpdateAgentMessageText(echo)` via `conn.SessionUpdate`; return `StopReasonEndTurn`

- [ ] **Step 1: Write failing pipe-based ACP turn test**

Create `internal/agent/agent_test.go`:

```go
package agent_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/agent"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type captureClient struct {
	mu      sync.Mutex
	chunks  []string
	updates chan struct{}
}

func (c *captureClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	u := params.Update
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		c.mu.Lock()
		c.chunks = append(c.chunks, u.AgentMessageChunk.Content.Text.Text)
		c.mu.Unlock()
		select {
		case c.updates <- struct{}{}:
		default:
		}
	}
	return nil
}

func (c *captureClient) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, nil
}
func (c *captureClient) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, nil
}
func (c *captureClient) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, nil
}
func (c *captureClient) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, nil
}
func (c *captureClient) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, nil
}
func (c *captureClient) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}
func (c *captureClient) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}
func (c *captureClient) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, nil
}

func TestEchoTurnOverPipes(t *testing.T) {
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	store := runtime.NewStore()
	ag := agent.New(store)
	asc := acp.NewAgentSideConnection(ag, agentToClientW, clientToAgentR)
	ag.SetAgentConnection(asc)

	client := &captureClient{updates: make(chan struct{}, 8)}
	csc := acp.NewClientSideConnection(client, clientToAgentW, agentToClientR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{Cwd: "/", McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		client.mu.Lock()
		joined := strings.Join(client.chunks, "")
		client.mu.Unlock()
		if joined == "hello" {
			return
		}
		select {
		case <-client.updates:
		case <-deadline:
			t.Fatalf("echo chunks = %q, want hello", joined)
		}
	}
}
```

If the SDK’s `Client` interface requires additional methods in v0.13.5, add stubs returning empty values / `MethodNotFound` until `var _ acp.Client = (*captureClient)(nil)` compiles.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/ -v
```

Expected: FAIL — `agent.New` undefined.

- [ ] **Step 3: Implement Agent core**

Create `internal/agent/agent.go`:

```go
package agent

import (
	"context"
	"fmt"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type Agent struct {
	store *runtime.Store

	mu   sync.Mutex
	conn *acp.AgentSideConnection
}

func New(store *runtime.Store) *Agent {
	return &Agent{store: store}
}

func (a *Agent) SetAgentConnection(conn *acp.AgentSideConnection) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.conn = conn
}

func (a *Agent) connection() *acp.AgentSideConnection {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn
}

func (a *Agent) Initialize(ctx context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
		},
	}, nil
}

func (a *Agent) NewSession(ctx context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	id, err := a.store.Create(runtime.EchoDefinition())
	if err != nil {
		return acp.NewSessionResponse{}, err
	}
	return acp.NewSessionResponse{SessionId: acp.SessionId(id)}, nil
}

func (a *Agent) Authenticate(ctx context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
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
	text := provider.Echo(provider.PromptText(params.Prompt))
	if err := conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: params.SessionId,
		Update:    acp.UpdateAgentMessageText(text),
	}); err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *Agent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	return nil
}

func (a *Agent) CloseSession(ctx context.Context, params acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	a.store.Delete(string(params.SessionId))
	return acp.CloseSessionResponse{}, nil
}
```

- [ ] **Step 4: Implement remaining Agent interface stubs**

Create `internal/agent/stubs.go` by copying the unused-method pattern from the SDK example (`example/agent/main.go` at tag `v0.13.5`): every required `acp.Agent` method not implemented in `agent.go` must exist and typically return `acp.NewMethodNotFound(<AgentMethod…>)` or an empty response.

Add compile-time assertion in `agent.go` or `stubs.go`:

```go
var _ acp.Agent = (*Agent)(nil)
```

Fix until `go test ./internal/agent/` compiles.

- [ ] **Step 5: Run agent tests**

```bash
go test ./internal/agent/ -v
```

Expected: PASS (`TestEchoTurnOverPipes`).

- [ ] **Step 6: Commit**

```bash
git add internal/agent
git commit -m "$(cat <<'EOF'
Implement ACP echo agent over SDK pipe connections.

EOF
)"
```

---

### Task 4: WebSocket NDJSON bridge

**Files:**
- Create: `internal/transport/ws/bridge.go`
- Create: `internal/transport/ws/bridge_test.go`

**Interfaces:**
- Consumes: `github.com/gorilla/websocket`
- Produces:
  - `ws.Bridge` embedding or wrapping `*websocket.Conn`
  - `ws.NewBridge(conn *websocket.Conn) *Bridge` implementing `io.Reader`, `io.Writer`, and `Close() error`
  - Framing rules:
    - **Write:** treat bytes as NDJSON; each complete line (trim `\n`) → one WebSocket **Text** message
    - **Read:** each WebSocket Text/Binary message → bytes + trailing `\n` for the SDK scanner
  - Concurrent Read/Write safe enough for SDK (mutex on WS write)

- [ ] **Step 1: Write failing bridge framing tests**

Use a `httptest` server that upgrades and echoes, or a net.Pipe-style approach with gorilla’s `websocket.NewClient` against `httptest`.

Create `internal/transport/ws/bridge_test.go`:

```go
package ws_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

func TestBridgeRoundTripLine(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		bridge := wstransport.NewBridge(c)
		// Agent-side: read one NDJSON line, write one back.
		line, err := bufio.NewReader(bridge).ReadString('\n')
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if _, err := io.WriteString(bridge, strings.TrimSpace(line)+"\n"); err != nil {
			t.Errorf("server write: %v", err)
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := wstransport.NewBridge(conn)
	if _, err := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatalf("client write: %v", err)
	}
	got, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if strings.TrimSpace(got) != `{"jsonrpc":"2.0","id":1,"method":"ping"}` {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/transport/ws/ -v
```

Expected: FAIL — `NewBridge` undefined.

- [ ] **Step 3: Implement bridge**

Create `internal/transport/ws/bridge.go`:

```go
package ws

import (
	"bytes"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

// Bridge adapts a WebSocket connection to newline-delimited JSON io.Reader/Writer
// expected by github.com/coder/acp-go-sdk.
type Bridge struct {
	conn *websocket.Conn

	readMu  sync.Mutex
	readBuf bytes.Buffer

	writeMu sync.Mutex
	wbuf    bytes.Buffer
}

func NewBridge(conn *websocket.Conn) *Bridge {
	return &Bridge{conn: conn}
}

func (b *Bridge) Read(p []byte) (int, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()

	for b.readBuf.Len() == 0 {
		_, data, err := b.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		b.readBuf.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			b.readBuf.WriteByte('\n')
		}
	}
	return b.readBuf.Read(p)
}

func (b *Bridge) Write(p []byte) (int, error) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	n, _ := b.wbuf.Write(p)
	for {
		chunk, err := b.wbuf.ReadBytes('\n')
		if err == io.EOF {
			// put partial line back
			b.wbuf.Write(chunk)
			break
		}
		if err != nil {
			return n, err
		}
		msg := bytes.TrimSuffix(chunk, []byte{'\n'})
		if err := b.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (b *Bridge) Close() error {
	return b.conn.Close()
}
```

Fix any race/partial-line bugs revealed by the test; keep the framing contract above.

- [ ] **Step 4: Run bridge tests**

```bash
go test ./internal/transport/ws/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/ws
git commit -m "$(cat <<'EOF'
Add WebSocket NDJSON bridge for ACP SDK framing.

EOF
)"
```

---

### Task 5: HTTP server, `/acp` handler, WebSocket integration test

**Files:**
- Create: `internal/transport/ws/handler.go`
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`
- Create: `cmd/controlplane/main.go`

**Interfaces:**
- Consumes: `agent.New`, `runtime.NewStore`, `ws.NewBridge`, `acp.NewAgentSideConnection`
- Produces:
  - `ws.Handler(store *runtime.Store) http.Handler` — upgrades `/acp`, creates per-connection Agent + Bridge, blocks until `asc.Done()`
  - `server.New(addr string, store *runtime.Store) *http.Server` — mux with `/acp` → handler
  - `cmd/controlplane` listens on `-addr` (default `:8080`)

- [ ] **Step 1: Write failing WebSocket ACP integration test**

Create `internal/server/server_test.go`:

```go
package server_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

type captureClient struct {
	mu     sync.Mutex
	chunks []string
}

func (c *captureClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	u := params.Update
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		c.mu.Lock()
		c.chunks = append(c.chunks, u.AgentMessageChunk.Content.Text.Text)
		c.mu.Unlock()
	}
	return nil
}

// Stub the rest of acp.Client exactly as in internal/agent/agent_test.go
// (RequestPermission, Read/WriteTextFile, terminal methods).

func TestWebSocketEchoTurn(t *testing.T) {
	store := runtime.NewStore()
	srv := httptest.NewServer(server.NewMux(store))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/acp"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	bridge := wstransport.NewBridge(conn)
	client := &captureClient{}
	csc := acp.NewClientSideConnection(client, bridge, bridge)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{Cwd: "/", McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client.mu.Lock()
		joined := strings.Join(client.chunks, "")
		client.mu.Unlock()
		if joined == "hello" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	t.Fatalf("chunks=%q want hello", strings.Join(client.chunks, ""))
}
```

Note: `NewClientSideConnection(client, bridge, bridge)` uses the same duplex bridge for peer input/output — this matches bidirectional WS. If the SDK assumes distinct readers/writers and deadlocks, split with `io.Pipe` pairs feeding the bridge’s Read/Write in goroutines; document the working pattern in the handler the same way.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/server/ -v
```

Expected: FAIL — `server.NewMux` undefined.

- [ ] **Step 3: Implement WS handler and server mux**

Create `internal/transport/ws/handler.go`:

```go
package ws

import (
	"log/slog"
	"net/http"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/tryy3/agent-fabric/internal/agent"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // local-dev only; tighten later with auth
}

func Handler(store *runtime.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("websocket upgrade", "err", err)
			return
		}
		bridge := NewBridge(conn)
		ag := agent.New(store)
		asc := acp.NewAgentSideConnection(ag, bridge, bridge)
		ag.SetAgentConnection(asc)
		asc.SetLogger(slog.Default())
		<-asc.Done()
		_ = bridge.Close()
	})
}
```

Create `internal/server/server.go`:

```go
package server

import (
	"net/http"

	"github.com/tryy3/agent-fabric/internal/runtime"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

func NewMux(store *runtime.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store))
	return mux
}

func New(addr string, store *runtime.Store) *http.Server {
	return &http.Server{Addr: addr, Handler: NewMux(store)}
}
```

Create `cmd/controlplane/main.go`:

```go
package main

import (
	"flag"
	"log"
	"log/slog"

	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	store := runtime.NewStore()
	srv := server.New(*addr, store)
	slog.Info("controlplane listening", "addr", *addr, "acp", "/acp")
	log.Fatal(srv.ListenAndServe())
}
```

- [ ] **Step 4: Run integration test**

```bash
go test ./internal/server/ -v
```

Expected: PASS.

- [ ] **Step 5: Smoke-build the server binary**

```bash
go build -o /tmp/controlplane ./cmd/controlplane
```

Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/transport/ws/handler.go internal/server cmd/controlplane
git commit -m "$(cat <<'EOF'
Serve ACP echo agent over WebSocket /acp.

EOF
)"
```

---

### Task 6: `acp-cli` and README

**Files:**
- Create: `cmd/acp-cli/main.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: WS bridge + `acp.NewClientSideConnection`
- Produces: CLI flags `-addr` (default `localhost:8080`), `-prompt` (default `hello`); prints streamed agent text to stdout

- [ ] **Step 1: Implement CLI**

Create `cmd/acp-cli/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

type printClient struct{}

func (printClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	u := params.Update
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		fmt.Print(u.AgentMessageChunk.Content.Text.Text)
	}
	return nil
}

// Stub remaining acp.Client methods like the agent tests (empty / nil-safe).

func main() {
	addr := flag.String("addr", "localhost:8080", "control plane host:port")
	prompt := flag.String("prompt", "hello", "user prompt text")
	flag.Parse()

	wsURL := "ws://" + *addr + "/acp"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	bridge := wstransport.NewBridge(conn)
	csc := acp.NewClientSideConnection(printClient{}, bridge, bridge)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		log.Fatalf("initialize: %v", err)
	}
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{Cwd: mustCwd(), McpServers: []acp.McpServer{}})
	if err != nil {
		log.Fatalf("session/new: %v", err)
	}
	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(*prompt)},
	}); err != nil {
		log.Fatalf("prompt: %v", err)
	}
	fmt.Println()
}

func mustCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "/"
	}
	return wd
}
```

Complete `acp.Client` stubs so it compiles (`var _ acp.Client = printClient{}`).

- [ ] **Step 2: Build CLI**

```bash
go build -o /tmp/acp-cli ./cmd/acp-cli
```

Expected: exit 0.

- [ ] **Step 3: Manual smoke (optional if server can bind)**

```bash
go run ./cmd/controlplane &
sleep 1
go run ./cmd/acp-cli -addr localhost:8080 -prompt "hello"
kill %1
```

Expected: stdout contains `hello`.

- [ ] **Step 4: Update README**

Replace the “design phase / implementation has not started” wording with a short **Run** section:

```markdown
## Run (control plane echo slice)

Requirements: Nix direnv shell (provides Go) or a local Go 1.22+ toolchain.

```bash
# terminal 1
go run ./cmd/controlplane

# terminal 2
go run ./cmd/acp-cli -addr localhost:8080 -prompt "hello"
```

Tests (offline, no API keys):

```bash
go test ./...
```

Design: [`docs/superpowers/specs/2026-09-11-controlplane-acp-echo-design.md`](docs/superpowers/specs/2026-09-11-controlplane-acp-echo-design.md).
```

Keep architecture/decisions links.

- [ ] **Step 5: Full test suite**

```bash
go test ./...
```

Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/acp-cli README.md
git commit -m "$(cat <<'EOF'
Add acp-cli smoke client and document how to run the echo slice.

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Go module + Nix Go tooling | Task 1 |
| Hardcoded echo definition / session pin | Task 2 |
| Scripted echo provider | Task 2 |
| ACP Agent via coder SDK | Task 3 |
| WebSocket `/acp` NDJSON bridge | Task 4 |
| Server + integration test | Task 5 |
| `cmd/controlplane` | Task 5 |
| `cmd/acp-cli` | Task 6 |
| README run instructions | Task 6 |
| Offline tests / no API keys | Tasks 2–5 |
| Out of scope (catalog, auth, Flutter, LLM, stdio) | Not scheduled |

## Self-review notes

- No TBD/placeholder steps; SDK field names for `ContentBlock` / `Client` stubs may need tiny compile-time adjustments — plan calls that out explicitly.
- Duplex `NewClientSideConnection(..., bridge, bridge)` is the intended pattern; Task 5 documents the pipe-split fallback if deadlock appears.
- `CheckOrigin: true` is intentional for local-dev only; auth/origin tightening is deferred with the rest of auth.
