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
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type captureClient struct {
	mu       sync.Mutex
	chunks   []string
	thoughts []string
	usages   []acp.SessionUsageUpdate
	updates  chan struct{}
}

func (c *captureClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	u := params.Update
	c.mu.Lock()
	if u.AgentThoughtChunk != nil && u.AgentThoughtChunk.Content.Text != nil {
		c.thoughts = append(c.thoughts, u.AgentThoughtChunk.Content.Text.Text)
	}
	if u.UsageUpdate != nil {
		c.usages = append(c.usages, *u.UsageUpdate)
	}
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		c.chunks = append(c.chunks, u.AgentMessageChunk.Content.Text.Text)
		c.mu.Unlock()
		select {
		case c.updates <- struct{}{}:
		default:
		}
		return nil
	}
	c.mu.Unlock()
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

func (r *recordingStreamer) StreamChat(ctx context.Context, model string, messages []runtime.Message, _ provider.StreamChatOptions, onEvent func(provider.StreamEvent) error) error {
	r.lastModel = model
	for _, c := range r.chunks {
		if err := onEvent(provider.StreamEvent{Content: c}); err != nil {
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
	streamFn     func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error
}

func (f *fakeStreamer) StreamChat(ctx context.Context, model string, messages []runtime.Message, _ provider.StreamChatOptions, onEvent func(provider.StreamEvent) error) error {
	f.mu.Lock()
	f.lastMessages = append([]runtime.Message(nil), messages...)
	fn := f.streamFn
	deltas := append([]string(nil), f.deltas...)
	err := f.err
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, model, messages, onEvent)
	}
	for _, d := range deltas {
		if e := onEvent(provider.StreamEvent{Content: d}); e != nil {
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
	ctx := context.Background()
	cat := catalog.Open(dbtest.Open(t))
	p, err := cat.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:8888/v1", "sk-test")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if len(models) > 0 {
		if _, err := cat.ReplaceProviderModels(ctx, p.ID, models, time.Now().UTC()); err != nil {
			t.Fatalf("ReplaceProviderModels: %v", err)
		}
	}
	var ag catalog.Agent
	if defaultModel != "" {
		ag, err = cat.CreateAgent(ctx, "Coder", "", p.ID, defaultModel)
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
	if pinned.Pin.ProviderName != "Local" {
		t.Fatalf("ProviderName = %q, want Local", pinned.Pin.ProviderName)
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

func TestSetConfigOptionPersistsThreadModel(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.SetSessionConfigOption(ctx2, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			ConfigId:  acp.SessionConfigId("model"),
			SessionId: sess.SessionId,
			Value:     acp.SessionConfigValueId("m2"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentModel == nil || *got.CurrentModel != "m2" {
		t.Fatalf("current_model = %v", got.CurrentModel)
	}
	sess2, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	live, ok := rt.Get(string(sess2.SessionId))
	if !ok || live.Pin.CurrentModel != "m2" {
		t.Fatalf("reopen model = %+v ok=%v", live.Pin, ok)
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

func TestNewSessionKeepsModelsWhenReplaceWouldOrphanDefault(t *testing.T) {
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	ctx := context.Background()
	if catalogAgent.ProviderID == nil {
		t.Fatal("expected seeded provider")
	}
	p, err := cat.GetProvider(ctx, *catalogAgent.ProviderID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if _, err := cat.ReplaceProviderModels(ctx, p.ID, nil, time.Now().UTC()); err == nil {
		t.Fatal("expected error when clearing models still referenced by agent")
	}
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})

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
	pinned, ok := store.Get(string(sess.SessionId))
	if !ok || pinned.Pin.CurrentModel != "m1" {
		t.Fatalf("pin = %+v ok=%v", pinned.Pin, ok)
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
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
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

func TestNewSessionRejectsWhenAlreadyClosed(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.New(store, cat)
	ag.CloseConnectionSessions()
	_, err = ag.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": th.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "connection closed") {
		t.Fatalf("err = %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("store len = %d", store.Len())
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != nil {
		t.Fatalf("closed session/new pinned agent %v", got.AgentID)
	}
}

func TestCloseConnectionSessionsAbortsInFlightPrompt(t *testing.T) {
	store := runtime.NewStore()
	started := make(chan struct{})
	fs := &fakeStreamer{
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
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
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
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

func TestNewSessionRejectsIncompleteAgent(t *testing.T) {
	store := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	if ag.ProviderID == nil {
		t.Fatal("expected seeded provider")
	}
	if err := cat.DeleteProvider(context.Background(), *ag.ProviderID); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx, cancel := startACPCatalog(t, store, cat, &fakeStreamer{})
	defer cancel()
	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": ag.ID},
	})
	if err == nil {
		t.Fatal("expected error for incomplete agent")
	}
}

func TestNewSessionWithThreadHydratesAndPinsAgent(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.CommitTurn(ctx, th.ID, "hello there", catalog.AssistantTurn{Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	live, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("runtime session missing")
	}
	if live.ThreadID != th.ID {
		t.Fatalf("ThreadID = %q", live.ThreadID)
	}
	if len(live.Messages) != 2 || live.Messages[0].Content != "hello there" {
		t.Fatalf("hydrated %+v", live.Messages)
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID == nil || *got.AgentID != catalogAgent.ID {
		t.Fatalf("pinned agent %v", got.AgentID)
	}
}

func TestNewSessionThreadAgentMismatchFails(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, a1 := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	if a1.ProviderID == nil {
		t.Fatal("expected seeded provider")
	}
	a2, err := cat.CreateAgent(ctx, "Other", "", *a1.ProviderID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.PinThreadAgent(ctx, th.ID, a1.ID); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err = csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": a2.ID, "threadId": th.ID},
	})
	if err == nil {
		t.Fatal("expected lock error")
	}
	if store.Len() != 0 {
		t.Fatalf("mismatch left runtime session store len = %d", store.Len())
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID == nil || *got.AgentID != a1.ID {
		t.Fatalf("agent pin = %v, want %s", got.AgentID, a1.ID)
	}
}

func TestNewSessionWithoutThreadIdDoesNotWriteThread(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	if _, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != nil {
		t.Fatalf("unbound session pinned thread: %v", got.AgentID)
	}
}

func TestNewSessionMissingThreadFails(t *testing.T) {
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": "th_missing"},
	})
	if err == nil {
		t.Fatal("expected missing thread error")
	}
}

func TestBoundPromptCommitsBothAndAutoTitles(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, &fakeStreamer{deltas: []string{"hello"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("How do I pin an agent to a thread please")},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title != "How do I pin an agent to a" {
		t.Fatalf("title %q", detail.Title)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages %d", len(detail.Messages))
	}
	live, _ := rt.Messages(string(sess.SessionId))
	if len(live) != 2 {
		t.Fatalf("runtime messages %d", len(live))
	}
}

func TestBoundPromptCancelWritesNothing(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	streamer := &fakeStreamer{streamFn: func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	agnt, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, streamer)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := csc.Prompt(ctx2, acp.PromptRequest{
			SessionId: sess.SessionId,
			Prompt:    []acp.ContentBlock{acp.TextBlock("will cancel")},
		})
		errCh <- err
	}()
	<-started
	if err := agnt.Cancel(context.Background(), acp.CancelNotification{SessionId: sess.SessionId}); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err == nil {
		t.Fatal("expected prompt error")
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("persisted %d messages", len(detail.Messages))
	}
	live, _ := rt.Messages(string(sess.SessionId))
	if len(live) != 0 {
		t.Fatalf("runtime %d", len(live))
	}
}

func TestUnboundPromptStillDoesNotTouchThreads(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, &fakeStreamer{deltas: []string{"echo"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess := mustNewSession(t, ctx2, csc, ag.ID)
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("unbound prompt wrote thread: %+v", detail.Messages)
	}
}

func TestThoughtAndUsageOverACPAndCommit(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pt := 3
	pps := 35.5
	ttft := int64(10)
	elapsed := int64(50)
	fs := &fakeStreamer{
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			if err := onEvent(provider.StreamEvent{Thought: "why "}); err != nil {
				return err
			}
			if err := onEvent(provider.StreamEvent{Thought: "me"}); err != nil {
				return err
			}
			if err := onEvent(provider.StreamEvent{Content: "hi", Finish: "stop"}); err != nil {
				return err
			}
			return onEvent(provider.StreamEvent{Usage: &provider.Usage{
				PromptTokens:       &pt,
				PredictedPerSecond: &pps,
				TTFTMs:             &ttft,
				ElapsedMs:          &elapsed,
				Deltas:             1,
			}})
		},
	}
	_, csc, client, ctx2, _ := startACPCatalog(t, rt, cat, fs)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("ask")},
	}); err != nil {
		t.Fatal(err)
	}

	client.mu.Lock()
	thoughts := strings.Join(client.thoughts, "")
	chunks := strings.Join(client.chunks, "")
	usages := append([]acp.SessionUsageUpdate(nil), client.usages...)
	client.mu.Unlock()
	if thoughts != "why me" {
		t.Fatalf("thoughts = %q", thoughts)
	}
	if chunks != "hi" {
		t.Fatalf("chunks = %q", chunks)
	}
	if len(usages) != 1 {
		t.Fatalf("usages = %d", len(usages))
	}
	if usages[0].Meta["predictedPerSecond"] != 35.5 {
		t.Fatalf("predictedPerSecond = %#v", usages[0].Meta["predictedPerSecond"])
	}
	if usages[0].Meta["stopReason"] != "end_turn" {
		t.Fatalf("stopReason = %#v", usages[0].Meta["stopReason"])
	}

	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	as := detail.Messages[1]
	if len(as.Parts) < 3 || as.Parts[0].Type != "thought" || as.Parts[1].Type != "message" || as.Parts[1].Text != "hi" {
		t.Fatalf("parts = %+v", as.Parts)
	}
	usage := as.Parts[2]
	if usage.Type != "usage" {
		t.Fatalf("usage part type = %q", usage.Type)
	}
	if usage.PromptTokens == nil || *usage.PromptTokens != pt {
		t.Fatalf("usage PromptTokens = %v", usage.PromptTokens)
	}
	if usage.PredictedPerSecond == nil || *usage.PredictedPerSecond != pps {
		t.Fatalf("usage PredictedPerSecond = %v", usage.PredictedPerSecond)
	}
	if usage.TTFTMs == nil || *usage.TTFTMs != ttft {
		t.Fatalf("usage TTFTMs = %v", usage.TTFTMs)
	}
	if usage.Deltas == nil || *usage.Deltas != 1 {
		t.Fatalf("usage Deltas = %v", usage.Deltas)
	}
	if usage.ElapsedMs == nil || *usage.ElapsedMs != elapsed {
		t.Fatalf("usage ElapsedMs = %v", usage.ElapsedMs)
	}
	live, ok := rt.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("session missing")
	}
	if as.Model == nil || *as.Model != live.Pin.CurrentModel {
		t.Fatalf("model = %v want %s", as.Model, live.Pin.CurrentModel)
	}
	if as.ProviderName == nil || *as.ProviderName != "Local" {
		t.Fatalf("providerName = %v", as.ProviderName)
	}

	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("again")},
	}); err != nil {
		t.Fatal(err)
	}
	got := fs.snapshotMessages()
	var asst string
	for _, m := range got {
		if m.Role == "assistant" {
			asst = m.Content
			break
		}
	}
	if asst != "hi" {
		t.Fatalf("snapshot assistant = %q, messages = %+v", asst, got)
	}
}

func TestMaxTokensStillCommits(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeStreamer{
		streamFn: func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			return onEvent(provider.StreamEvent{Content: "cut", Finish: "length"})
		},
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, fs)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("ask")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != acp.StopReasonMaxTokens {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, acp.StopReasonMaxTokens)
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	as := detail.Messages[1]
	if as.StopReason == nil || *as.StopReason != "max_tokens" {
		t.Fatalf("row StopReason = %v", as.StopReason)
	}
}
