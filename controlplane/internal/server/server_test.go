package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

type captureClient struct {
	mu      sync.Mutex
	chunks  []string
	updates chan struct{}
}

func (c *captureClient) SessionUpdate(_ context.Context, params acp.SessionNotification) error {
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

var _ acp.Client = (*captureClient)(nil)

func TestCatalogHTTPMountedAlongsideACP(t *testing.T) {
	cat := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(server.NewMux(runtime.NewStore(), cat))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/providers status %d body %s", resp.StatusCode, body)
	}
}

func TestCatalogCORSPreflightAndGET(t *testing.T) {
	cat := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(server.NewMux(runtime.NewStore(), cat))
	defer srv.Close()

	const origin = "http://localhost:54321"
	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/v1/providers", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Allow-Origin = %q, want %q", got, origin)
	}
	if !strings.Contains(resp.Header.Get("Access-Control-Allow-Methods"), "GET") {
		t.Fatalf("Allow-Methods = %q", resp.Header.Get("Access-Control-Allow-Methods"))
	}

	getReq, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/providers", nil)
	if err != nil {
		t.Fatal(err)
	}
	getReq.Header.Set("Origin", origin)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status %d", getResp.StatusCode)
	}
	if got := getResp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("GET Allow-Origin = %q, want %q", got, origin)
	}
}

func TestWebSocketStreamedTurn(t *testing.T) {
	var mu sync.Mutex
	var lastMessages []runtime.Message
	turn := 0
	openai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Messages []runtime.Message `json:"messages"`
		}
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		lastMessages = append([]runtime.Message(nil), body.Messages...)
		n := turn
		turn++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		if n == 0 {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
			flusher.Flush()
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		} else {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"c\"}}]}\n\n")
		}
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer openai.Close()

	store := runtime.NewStore()
	seedCtx := context.Background()
	cat := catalog.Open(dbtest.Open(t))
	p, err := cat.CreateProvider(seedCtx, "Local", catalog.TypeOpenAICompatible, openai.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.ReplaceProviderModels(seedCtx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	catalogAgent, err := cat.CreateAgent(seedCtx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.NewMux(store, cat))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/acp"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	bridge := wstransport.NewBridge(conn)
	client := &captureClient{updates: make(chan struct{}, 8)}
	csc := acp.NewClientSideConnection(client, bridge, bridge)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("a")},
	}); err != nil {
		t.Fatalf("Prompt a: %v", err)
	}
	waitJoined(t, client, "hello")

	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("b")},
	}); err != nil {
		t.Fatalf("Prompt b: %v", err)
	}

	mu.Lock()
	got := append([]runtime.Message(nil), lastMessages...)
	mu.Unlock()
	want := []runtime.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lastMessages = %+v, want %+v", got, want)
	}
}

func waitJoined(t *testing.T, client *captureClient, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		client.mu.Lock()
		joined := strings.Join(client.chunks, "")
		client.mu.Unlock()
		if joined == want {
			return
		}
		select {
		case <-client.updates:
		case <-deadline:
			t.Fatalf("stream chunks = %q, want %q", joined, want)
		}
	}
}
