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
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
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

type recordingStreamer struct {
	lastModel string
	chunks    []string
}

func (r *recordingStreamer) StreamChat(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
	r.lastModel = model
	for _, c := range r.chunks {
		if err := onDelta(c); err != nil {
			return err
		}
	}
	return nil
}

type fakeStreamer struct {
	mu           sync.Mutex
	deltas       []string
	err          error
	lastMessages []runtime.Message
	streamFn     func(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error
}

func (f *fakeStreamer) StreamChat(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
	f.mu.Lock()
	f.lastMessages = append([]runtime.Message(nil), messages...)
	fn := f.streamFn
	deltas := append([]string(nil), f.deltas...)
	err := f.err
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, model, messages, onDelta)
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

func seedCatalog(t *testing.T, models []catalog.ModelInfo, defaultModel string) (*catalog.Store, catalog.Agent) {
	t.Helper()
	cat, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatalf("catalog.Open: %v", err)
	}
	p, err := cat.CreateProvider("Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:8888/v1", "sk-test")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if len(models) > 0 {
		if _, err := cat.ReplaceProviderModels(p.ID, models, time.Now().UTC()); err != nil {
			t.Fatalf("ReplaceProviderModels: %v", err)
		}
	}
	var ag catalog.Agent
	if defaultModel != "" {
		ag, err = cat.CreateAgent("Coder", "", p.ID, defaultModel)
		if err != nil {
			t.Fatalf("CreateAgent: %v", err)
		}
	}
	return cat, ag
}

func startACP(t *testing.T, store *runtime.Store, streamer *fakeStreamer) (*agent.Agent, *acp.ClientSideConnection, *captureClient, context.Context, context.CancelFunc, catalog.Agent) {
	t.Helper()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	ag, csc, client, ctx, cancel := startACPCatalog(t, store, cat, streamer)
	return ag, csc, client, ctx, cancel, catalogAgent
}

func mustNewSession(t *testing.T, ctx context.Context, csc *acp.ClientSideConnection, agentID string) acp.NewSessionResponse {
	t.Helper()
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": agentID},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return sess
}

func startACPCatalog(t *testing.T, store *runtime.Store, cat *catalog.Store, streamer provider.ChatStreamer) (*agent.Agent, *acp.ClientSideConnection, *captureClient, context.Context, context.CancelFunc) {
	t.Helper()
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	ag := agent.New(store, cat)
	if streamer != nil {
		ag.SetTestStreamer(streamer)
	}
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

func TestNewSessionPinsCatalogAgentAndModelOptions(t *testing.T) {
	store := runtime.NewStore()
	models := []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}
	cat, catalogAgent := seedCatalog(t, models, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})

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
	if sess.SessionId == "" {
		t.Fatal("expected session id")
	}
	if len(sess.ConfigOptions) != 1 || sess.ConfigOptions[0].Select == nil {
		t.Fatalf("ConfigOptions = %+v, want one model select", sess.ConfigOptions)
	}
	sel := sess.ConfigOptions[0].Select
	if sel.Id != acp.SessionConfigId("model") {
		t.Fatalf("select id = %q, want model", sel.Id)
	}
	if sel.CurrentValue != acp.SessionConfigValueId("m1") {
		t.Fatalf("CurrentValue = %q, want m1", sel.CurrentValue)
	}
	if sel.Options.Ungrouped == nil || len(*sel.Options.Ungrouped) != 2 {
		t.Fatalf("options = %+v, want 2 ungrouped", sel.Options)
	}

	pinned, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("session not stored")
	}
	if pinned.Pin.AgentID != catalogAgent.ID || pinned.Pin.CurrentModel != "m1" || len(pinned.Pin.Models) != 2 {
		t.Fatalf("pin = %+v", pinned.Pin)
	}
}

func TestSetSessionConfigOptionSwitchesModel(t *testing.T) {
	store := runtime.NewStore()
	models := []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}
	cat, catalogAgent := seedCatalog(t, models, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	resp, err := csc.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			ConfigId:  acp.SessionConfigId("model"),
			SessionId: sess.SessionId,
			Value:     acp.SessionConfigValueId("m2"),
		},
	})
	if err != nil {
		t.Fatalf("SetSessionConfigOption: %v", err)
	}
	if len(resp.ConfigOptions) != 1 || resp.ConfigOptions[0].Select == nil {
		t.Fatalf("ConfigOptions = %+v, want one model select", resp.ConfigOptions)
	}
	if got := resp.ConfigOptions[0].Select.CurrentValue; got != acp.SessionConfigValueId("m2") {
		t.Fatalf("CurrentValue = %q, want m2", got)
	}

	pinned, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("session not stored")
	}
	if pinned.Pin.CurrentModel != "m2" {
		t.Fatalf("pin CurrentModel = %q, want m2", pinned.Pin.CurrentModel)
	}
}

func TestSetSessionConfigOptionRejectsUnknownModel(t *testing.T) {
	store := runtime.NewStore()
	models := []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}
	cat, catalogAgent := seedCatalog(t, models, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	_, err := csc.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			ConfigId:  acp.SessionConfigId("model"),
			SessionId: sess.SessionId,
			Value:     acp.SessionConfigValueId("m3"),
		},
	})
	if err == nil {
		t.Fatal("expected error for unknown model")
	}

	pinned, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("session not stored")
	}
	if pinned.Pin.CurrentModel != "m1" {
		t.Fatalf("pin CurrentModel = %q, want m1 unchanged", pinned.Pin.CurrentModel)
	}
}

func TestSetSessionConfigOptionRejectsUnknownConfig(t *testing.T) {
	store := runtime.NewStore()
	models := []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}
	cat, catalogAgent := seedCatalog(t, models, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	_, err := csc.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			ConfigId:  acp.SessionConfigId("temperature"),
			SessionId: sess.SessionId,
			Value:     acp.SessionConfigValueId("m2"),
		},
	})
	if err == nil {
		t.Fatal("expected error for unknown config option")
	}

	pinned, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("session not stored")
	}
	if pinned.Pin.CurrentModel != "m1" {
		t.Fatalf("pin CurrentModel = %q, want m1 unchanged", pinned.Pin.CurrentModel)
	}
}

func TestPromptUsesCurrentModelAfterSetConfigOption(t *testing.T) {
	store := runtime.NewStore()
	models := []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}
	cat, catalogAgent := seedCatalog(t, models, "m1")
	rec := &recordingStreamer{chunks: []string{"ok"}}
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, rec)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	if _, err := csc.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			ConfigId:  acp.SessionConfigId("model"),
			SessionId: sess.SessionId,
			Value:     acp.SessionConfigValueId("m2"),
		},
	}); err != nil {
		t.Fatalf("SetSessionConfigOption: %v", err)
	}

	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if rec.lastModel != "m2" {
		t.Fatalf("lastModel = %q, want m2", rec.lastModel)
	}
}

func TestNewSessionRequiresAgentId(t *testing.T) {
	store := runtime.NewStore()
	cat, _ := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err == nil {
		t.Fatal("expected error for missing agentId")
	}
}

func TestNewSessionRejectsEmptyModels(t *testing.T) {
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	p, ok := cat.GetProvider(catalogAgent.ProviderID)
	if !ok {
		t.Fatal("provider missing")
	}
	if _, err := cat.ReplaceProviderModels(p.ID, nil, time.Now().UTC()); err != nil {
		t.Fatalf("ReplaceProviderModels: %v", err)
	}
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID},
	})
	if err == nil {
		t.Fatal("expected error for empty models")
	}
}

func TestStreamedTurnOverPipes(t *testing.T) {
	store := runtime.NewStore()
	fs := &fakeStreamer{deltas: []string{"Hel", "lo"}}
	_, csc, client, ctx, _, catalogAgent := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)
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
	_, csc, _, ctx, _, catalogAgent := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)
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
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	ag, csc, _, ctx, _, catalogAgent := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	go func() {
		<-started
		_ = ag.Cancel(context.Background(), acp.CancelNotification{SessionId: sess.SessionId})
	}()
	_, err := csc.Prompt(ctx, acp.PromptRequest{
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

func TestCloseConnectionSessionsAbortsInFlightPrompt(t *testing.T) {
	store := runtime.NewStore()
	started := make(chan struct{})
	fs := &fakeStreamer{
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	ag, csc, _, ctx, _, catalogAgent := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	errCh := make(chan error, 1)
	go func() {
		_, e := csc.Prompt(ctx, acp.PromptRequest{
			SessionId: sess.SessionId,
			Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
		})
		errCh <- e
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt never started streaming")
	}

	ag.CloseConnectionSessions()

	select {
	case e := <-errCh:
		if e == nil {
			t.Fatal("expected prompt error after CloseConnectionSessions")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt still running after CloseConnectionSessions")
	}

	msgs, ok := store.Messages(string(sess.SessionId))
	if ok {
		for _, m := range msgs {
			if m.Role == "assistant" {
				t.Fatalf("history = %+v, want no assistant", msgs)
			}
		}
	}
}

func TestOverlappingPromptKeepsLiveCancel(t *testing.T) {
	store := runtime.NewStore()
	started := make(chan struct{}, 2)
	fs := &fakeStreamer{
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	ag, csc, _, ctx, _, catalogAgent := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)

	req := func(text string) acp.PromptRequest {
		return acp.PromptRequest{
			SessionId: sess.SessionId,
			Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
		}
	}

	errA := make(chan error, 1)
	go func() {
		_, e := ag.Prompt(ctx, req("a"))
		errA <- e
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt A never started streaming")
	}

	errB := make(chan error, 1)
	go func() {
		_, e := ag.Prompt(ctx, req("b"))
		errB <- e
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt B never started streaming")
	}

	select {
	case e := <-errA:
		if e == nil {
			t.Fatal("prompt A should be cancelled by overlapping prompt B")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt A did not return after B started")
	}

	if err := ag.Cancel(context.Background(), acp.CancelNotification{SessionId: sess.SessionId}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case e := <-errB:
		if e == nil {
			t.Fatal("expected prompt B error after Cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt B still running after Cancel; live cancel was dropped")
	}
}

func TestEmptySuccessfulStreamDoesNotAppendAssistant(t *testing.T) {
	store := runtime.NewStore()
	fs := &fakeStreamer{deltas: nil}
	_, csc, _, ctx, _, catalogAgent := startACP(t, store, fs)

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess := mustNewSession(t, ctx, csc, catalogAgent.ID)
	_, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	})
	if err == nil {
		t.Fatal("expected error for empty successful stream")
	}
	msgs, _ := store.Messages(string(sess.SessionId))
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("history = %+v, want only user", msgs)
	}
}
