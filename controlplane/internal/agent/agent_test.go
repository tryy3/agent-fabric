package agent_test

import (
	"context"
	"io"
	"reflect"
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

var _ acp.Client = (*captureClient)(nil)

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

func (f *fakeStreamer) snapshotMessages() []runtime.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]runtime.Message, len(f.lastMessages))
	copy(out, f.lastMessages)
	return out
}

func startACP(t *testing.T, store *runtime.Store, streamer *fakeStreamer) (*agent.Agent, *acp.ClientSideConnection, *captureClient, context.Context, context.CancelFunc) {
	t.Helper()
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	ag := agent.New(store, streamer)
	asc := acp.NewAgentSideConnection(ag, agentToClientW, clientToAgentR)
	ag.SetAgentConnection(asc)

	client := &captureClient{updates: make(chan struct{}, 8)}
	csc := acp.NewClientSideConnection(client, clientToAgentW, agentToClientR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(func() {
		cancel()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})
	return ag, csc, client, ctx, cancel
}

func TestStreamedTurnOverPipes(t *testing.T) {
	store := runtime.NewStore()
	fs := &fakeStreamer{deltas: []string{"Hel", "lo"}}
	_, csc, client, ctx, _ := startACP(t, store, fs)

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
		if joined == "Hello" {
			return
		}
		select {
		case <-client.updates:
		case <-deadline:
			t.Fatalf("stream chunks = %q, want Hello", joined)
		}
	}
}

func TestMultiTurnSendsHistory(t *testing.T) {
	store := runtime.NewStore()
	fs := &fakeStreamer{deltas: []string{"yo"}}
	_, csc, _, ctx, _ := startACP(t, store, fs)

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
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	}); err != nil {
		t.Fatalf("Prompt hi: %v", err)
	}

	fs.mu.Lock()
	fs.deltas = []string{"ok"}
	fs.mu.Unlock()

	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("again")},
	}); err != nil {
		t.Fatalf("Prompt again: %v", err)
	}

	got := fs.snapshotMessages()
	want := []runtime.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "yo"},
		{Role: "user", Content: "again"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lastMessages = %+v, want %+v", got, want)
	}
}

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
	ag, csc, _, ctx, _ := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{Cwd: "/", McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	go func() {
		<-started
		_ = ag.Cancel(context.Background(), acp.CancelNotification{SessionId: sess.SessionId})
	}()
	_, err = csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	})
	if err == nil {
		t.Fatal("expected prompt error after cancel")
	}
	msgs, _ := store.Messages(string(sess.SessionId))
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("history = %+v, want only user", msgs)
	}
}
