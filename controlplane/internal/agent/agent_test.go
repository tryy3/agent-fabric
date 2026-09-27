package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

type captureClient struct {
	mu              sync.Mutex
	chunks          []string
	thoughts        []string
	usages          []acp.SessionUsageUpdate
	toolCalls       []acp.SessionUpdateToolCall
	toolCallUpdates []acp.SessionToolCallUpdate
	updates         chan struct{}
	permissionFn    func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
	elicitationFn   func(context.Context, acp.UnstableCreateElicitationRequest) (acp.UnstableCreateElicitationResponse, error)
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
	if u.ToolCall != nil {
		c.toolCalls = append(c.toolCalls, *u.ToolCall)
	}
	if u.ToolCallUpdate != nil {
		c.toolCallUpdates = append(c.toolCallUpdates, *u.ToolCallUpdate)
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

func (c *captureClient) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	c.mu.Lock()
	fn := c.permissionFn
	c.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.NewRequestPermissionOutcomeCancelled(),
	}, nil
}

func (c *captureClient) UnstableCreateElicitation(ctx context.Context, req acp.UnstableCreateElicitationRequest) (acp.UnstableCreateElicitationResponse, error) {
	c.mu.Lock()
	fn := c.elicitationFn
	c.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return acp.NewUnstableCreateElicitationResponseCancel(), nil
}

func (c *captureClient) UnstableCompleteElicitation(context.Context, acp.UnstableCompleteElicitationNotification) error {
	return nil
}
func (c *captureClient) UnstableConnectMcp(context.Context, acp.UnstableConnectMcpRequest) (acp.UnstableConnectMcpResponse, error) {
	return acp.UnstableConnectMcpResponse{}, acp.NewMethodNotFound(acp.ClientMethodMcpConnect)
}
func (c *captureClient) UnstableDisconnectMcp(context.Context, acp.UnstableDisconnectMcpRequest) (acp.UnstableDisconnectMcpResponse, error) {
	return acp.UnstableDisconnectMcpResponse{}, acp.NewMethodNotFound(acp.ClientMethodMcpDisconnect)
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
	options      []provider.StreamChatOptions
	streamFn     func(ctx context.Context, model string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error
}

func (f *fakeStreamer) StreamChat(ctx context.Context, model string, messages []runtime.Message, opts provider.StreamChatOptions, onEvent func(provider.StreamEvent) error) error {
	f.mu.Lock()
	f.lastMessages = append([]runtime.Message(nil), messages...)
	f.options = append(f.options, opts)
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
	return startACPCatalogWithSandbox(t, store, cat, streamer, sandboxconfig.Engine{DataDir: t.TempDir()})
}

func startACPCatalogWithSandbox(
	t *testing.T,
	store *runtime.Store,
	cat *catalog.Store,
	streamer provider.ChatStreamer,
	engine sandboxconfig.Engine,
) (*agent.Agent, *acp.ClientSideConnection, *captureClient, context.Context, context.CancelFunc) {
	t.Helper()
	if engine.DataDir == "" {
		engine.DataDir = t.TempDir()
	}
	if _, err := cat.EnsurePlaneSettings(context.Background(), catalog.DeprecatedSandbox{Kind: "local"}); err != nil {
		t.Fatal(err)
	}
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	ag := agent.New(store, cat, engine)
	if streamer != nil {
		ag.SetTestStreamer(streamer)
	}
	if err := linkGlobalTestResource(t, cat); err != nil {
		t.Fatal(err)
	}
	ag.SetTestEnvironment(localProjectSandbox(engine.DataDir))
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

func linkGlobalTestResource(t *testing.T, cat *catalog.Store) error {
	t.Helper()
	ctx := context.Background()
	spec, err := json.Marshal(map[string]any{
		"image":         "alpine:3.20",
		"containerName": "test-box",
		"volumes": []map[string]any{{
			"id":          "vol_0123456789abcdef",
			"enabled":     true,
			"name":        "test-disk",
			"target":      "/workspace",
			"whitelisted": true,
			"read":        true,
			"write":       true,
			"exec":        true,
		}},
	})
	if err != nil {
		return err
	}
	resource, err := cat.CreateResource(ctx, "test-box", catalog.KindContainer, spec)
	if err != nil {
		return err
	}
	env, err := json.Marshal(map[string]string{"resourceId": resource.ID})
	if err != nil {
		return err
	}
	_, err = cat.PatchPlaneSettings(ctx, nil, env)
	return err
}

func localProjectSandbox(dataDir string) func(context.Context, sandbox.OpenOptions) (sandbox.Environment, error) {
	return func(ctx context.Context, opts sandbox.OpenOptions) (sandbox.Environment, error) {
		root := dataDir
		if opts.Docker != nil && opts.Docker.Scope.ProjectID != "" {
			root = sandbox.ProjectWorkspaceRoot(dataDir, opts.Docker.Scope.ProjectID)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, err
		}
		policy := remapWorkspaceGrants(opts.PathPolicy, opts.WorkspaceRoot, root)
		return sandbox.Open(ctx, sandbox.OpenOptions{
			Kind:          "local",
			WorkspaceRoot: root,
			PathPolicy:    policy,
		})
	}
}

func remapWorkspaceGrants(policy *sandbox.PathPolicy, containerRoot, hostRoot string) *sandbox.PathPolicy {
	if policy == nil {
		return nil
	}
	out := make([]sandbox.PathGrant, 0, len(policy.Grants))
	for _, grant := range policy.Grants {
		g := grant
		if g.Path == containerRoot || g.Path == "/workspace" {
			g.Path = hostRoot
		}
		out = append(out, g)
	}
	return &sandbox.PathPolicy{Grants: out}
}

func TestPromptExecutesSandboxToolAndCommitsACPUpdates(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workspace := sandbox.ProjectWorkspaceRoot(root, th.ProjectID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace+"/test.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	round := 0
	firstPromptTokens := 3
	firstCompletionTokens := 2
	firstTotalTokens := 5
	firstPromptMs := 10.0
	firstPredictedMs := 20.0
	firstElapsedMs := int64(30)
	firstPromptRate := 300.0
	firstPredictedRate := 100.0
	secondPromptTokens := 7
	secondCompletionTokens := 11
	secondTotalTokens := 18
	secondPromptMs := 40.0
	secondPredictedMs := 50.0
	secondElapsedMs := int64(90)
	secondPromptRate := 175.0
	secondPredictedRate := 220.0
	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			round++
			if round == 1 {
				if err := onEvent(provider.StreamEvent{Thought: "plan read"}); err != nil {
					return err
				}
				return onEvent(provider.StreamEvent{
					Content: "working",
					Finish:  "tool_calls",
					ToolCalls: []provider.ToolCall{{
						ID:        "call_1",
						Name:      "read_file",
						Arguments: `{"path":"test.txt"}`,
					}},
					Usage: &provider.Usage{
						PromptTokens:       &firstPromptTokens,
						CompletionTokens:   &firstCompletionTokens,
						TotalTokens:        &firstTotalTokens,
						PromptMs:           &firstPromptMs,
						PredictedMs:        &firstPredictedMs,
						ElapsedMs:          &firstElapsedMs,
						PromptPerSecond:    &firstPromptRate,
						PredictedPerSecond: &firstPredictedRate,
						Deltas:             99,
					},
				})
			}
			if len(messages) != 3 {
				t.Fatalf("round 2 messages = %+v", messages)
			}
			if messages[1].Role != "assistant" || len(messages[1].ToolCalls) != 1 {
				t.Fatalf("assistant tool message = %+v", messages[1])
			}
			if messages[1].Content != "working" {
				t.Fatalf("tool-round assistant content = %q, want buffered round text", messages[1].Content)
			}
			if messages[2].Role != "tool" || messages[2].ToolCallID != "call_1" ||
				messages[2].Content != `{"content":"hello"}` {
				t.Fatalf("tool result message = %+v", messages[2])
			}
			if err := onEvent(provider.StreamEvent{Thought: "summarize"}); err != nil {
				return err
			}
			return onEvent(provider.StreamEvent{
				Content: "ok",
				Finish:  "stop",
				Usage: &provider.Usage{
					PromptTokens:       &secondPromptTokens,
					CompletionTokens:   &secondCompletionTokens,
					TotalTokens:        &secondTotalTokens,
					PromptMs:           &secondPromptMs,
					PredictedMs:        &secondPredictedMs,
					ElapsedMs:          &secondElapsedMs,
					PromptPerSecond:    &secondPromptRate,
					PredictedPerSecond: &secondPredictedRate,
					Deltas:             88,
				},
			})
		},
	}
	_, csc, client, ctx2, _ := startACPCatalogWithSandbox(
		t,
		rt,
		cat,
		fs,
		sandboxconfig.Engine{DataDir: root},
	)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("read test.txt")},
	}); err != nil {
		t.Fatal(err)
	}
	fs.mu.Lock()
	options := append([]provider.StreamChatOptions(nil), fs.options...)
	fs.mu.Unlock()
	if len(options) != 2 || len(options[0].Tools) != 3 || len(options[1].Tools) != 3 {
		t.Fatalf("stream options = %+v", options)
	}

	client.mu.Lock()
	starts := append([]acp.SessionUpdateToolCall(nil), client.toolCalls...)
	updates := append([]acp.SessionToolCallUpdate(nil), client.toolCallUpdates...)
	chunks := append([]string(nil), client.chunks...)
	client.mu.Unlock()
	if !reflect.DeepEqual(chunks, []string{"ok"}) {
		t.Fatalf("agent message chunks = %#v, want only final-round text", chunks)
	}
	wantRawInput := map[string]any{"path": "test.txt"}
	wantRawOutput := map[string]any{"content": "hello"}
	if len(starts) != 1 || starts[0].ToolCallId != "call_1" ||
		starts[0].Status != acp.ToolCallStatusPending ||
		!reflect.DeepEqual(starts[0].RawInput, wantRawInput) {
		t.Fatalf("tool starts = %+v", starts)
	}
	if len(updates) != 1 || updates[0].ToolCallId != "call_1" ||
		updates[0].Status == nil || *updates[0].Status != acp.ToolCallStatusCompleted ||
		!reflect.DeepEqual(updates[0].RawOutput, wantRawOutput) {
		t.Fatalf("tool updates = %+v", updates)
	}
	client.mu.Lock()
	usages := append([]acp.SessionUsageUpdate(nil), client.usages...)
	client.mu.Unlock()
	if len(usages) != 1 {
		t.Fatalf("usages = %+v", usages)
	}
	if usages[0].Used != 23 ||
		usages[0].Meta["promptTokens"] != float64(10) ||
		usages[0].Meta["completionTokens"] != float64(13) ||
		usages[0].Meta["totalTokens"] != float64(23) ||
		usages[0].Meta["promptMs"] != float64(50) ||
		usages[0].Meta["predictedMs"] != float64(70) ||
		usages[0].Meta["elapsedMs"] != float64(120) ||
		usages[0].Meta["deltas"] != float64(2) {
		t.Fatalf("aggregated ACP usage = %+v", usages[0])
	}
	if usages[0].Meta["ttftMs"] == nil {
		t.Fatalf("ACP usage missing first TTFT: %+v", usages[0])
	}
	if _, ok := usages[0].Meta["promptPerSecond"]; ok {
		t.Fatalf("ACP usage kept per-round prompt rate: %+v", usages[0])
	}
	if _, ok := usages[0].Meta["predictedPerSecond"]; ok {
		t.Fatalf("ACP usage kept per-round predicted rate: %+v", usages[0])
	}

	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Messages[1].Content != "ok" {
		t.Fatalf("CommitTurn content = %q, want final text only (no tool-round text)", detail.Messages[1].Content)
	}
	parts := detail.Messages[1].Parts
	if len(parts) != 5 {
		t.Fatalf("parts = %+v", parts)
	}
	if parts[0].Type != "thought" || parts[0].Text != "plan read" {
		t.Fatalf("first thought part = %+v", parts[0])
	}
	toolPart := parts[1]
	if toolPart.Type != "tool_call" || toolPart.ToolCallID != "call_1" ||
		toolPart.Name != "read_file" || toolPart.Input != `{"path":"test.txt"}` ||
		toolPart.Output != `{"content":"hello"}` || toolPart.Status != "completed" {
		t.Fatalf("tool part = %+v", toolPart)
	}
	if parts[2].Type != "thought" || parts[2].Text != "summarize" {
		t.Fatalf("second thought part = %+v", parts[2])
	}
	if parts[3].Type != "message" || parts[3].Text != "ok" || parts[4].Type != "usage" {
		t.Fatalf("parts = %+v", parts)
	}
	usage := parts[4]
	if usage.PromptTokens == nil || *usage.PromptTokens != 10 ||
		usage.CompletionTokens == nil || *usage.CompletionTokens != 13 ||
		usage.TotalTokens == nil || *usage.TotalTokens != 23 ||
		usage.PromptMs == nil || *usage.PromptMs != 50 ||
		usage.PredictedMs == nil || *usage.PredictedMs != 70 ||
		usage.ElapsedMs == nil || *usage.ElapsedMs != 120 ||
		usage.Deltas == nil || *usage.Deltas != 2 {
		t.Fatalf("aggregated committed usage = %+v", usage)
	}
	if usage.TTFTMs == nil {
		t.Fatalf("committed usage missing first TTFT: %+v", usage)
	}
	if usage.PromptPerSecond != nil || usage.PredictedPerSecond != nil {
		t.Fatalf("committed usage kept per-round rates: %+v", usage)
	}
}

func TestPromptIsolatesLocalProjectWorkspaces(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rt := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	projectA, err := cat.CreateProject(ctx, "Alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := cat.CreateProject(ctx, "Beta", "")
	if err != nil {
		t.Fatal(err)
	}
	threadA, err := cat.CreateThreadForProject(ctx, projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	threadB, err := cat.CreateThreadForProject(ctx, projectB.ID)
	if err != nil {
		t.Fatal(err)
	}

	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			last := messages[len(messages)-1]
			if last.Role == "tool" {
				return onEvent(provider.StreamEvent{Content: "done", Finish: "stop"})
			}
			lastUser := last.Content
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "user" {
					lastUser = messages[i].Content
					break
				}
			}
			call := provider.ToolCall{ID: "call_iso", Name: "read_file", Arguments: `{"path":"secret.txt"}`}
			if strings.Contains(lastUser, "write") {
				call = provider.ToolCall{
					ID:        "call_iso",
					Name:      "write_file",
					Arguments: `{"path":"secret.txt","content":"from-a"}`,
				}
			}
			return onEvent(provider.StreamEvent{
				Finish:    "tool_calls",
				ToolCalls: []provider.ToolCall{call},
			})
		},
	}
	_, csc, _, ctx2, _ := startACPCatalogWithSandbox(
		t,
		rt,
		cat,
		fs,
		sandboxconfig.Engine{DataDir: root},
	)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sessA, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": threadA.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessB, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": threadB.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sessA.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("write secret")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sessB.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("read secret")},
	}); err != nil {
		t.Fatal(err)
	}

	pathA := filepath.Join(sandbox.ProjectWorkspaceRoot(root, projectA.ID), "secret.txt")
	pathB := filepath.Join(sandbox.ProjectWorkspaceRoot(root, projectB.ID), "secret.txt")
	got, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-a" {
		t.Fatalf("project A file = %q", got)
	}
	if _, err := os.Stat(pathB); !os.IsNotExist(err) {
		t.Fatalf("project B saw project A's file: %v", err)
	}

	detailB, err := cat.GetThread(ctx, threadB.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundReadError := false
	for _, msg := range detailB.Messages {
		for _, part := range msg.Parts {
			if part.Type == "tool_call" && strings.Contains(part.Output, "error") {
				foundReadError = true
			}
		}
	}
	if !foundReadError {
		t.Fatalf("project B read_file should not see project A: %+v", detailB.Messages)
	}
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
	ag := agent.New(store, cat, sandboxconfig.Engine{DataDir: t.TempDir()})
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
	emittedTTFT, ok := usages[0].Meta["ttftMs"].(float64)
	if !ok {
		t.Fatalf("ttftMs = %#v", usages[0].Meta["ttftMs"])
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
	if usage.TTFTMs == nil || float64(*usage.TTFTMs) != emittedTTFT {
		t.Fatalf("usage TTFTMs = %v, ACP ttftMs = %v", usage.TTFTMs, emittedTTFT)
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

func TestPromptPermissionAllowOnceElevatesPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("classified"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := runtime.NewStore()
	cat, agDef := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workspace := sandbox.ProjectWorkspaceRoot(root, th.ProjectID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	round := 0
	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			round++
			if round == 1 {
				return onEvent(provider.StreamEvent{
					Finish: "tool_calls",
					ToolCalls: []provider.ToolCall{{
						ID:        "call_perm",
						Name:      "read_file",
						Arguments: fmt.Sprintf(`{"path":%q}`, secret),
					}},
				})
			}
			if len(messages) < 3 || messages[2].Role != "tool" {
				t.Fatalf("messages = %+v", messages)
			}
			if !strings.Contains(messages[2].Content, "classified") {
				t.Fatalf("tool result = %s", messages[2].Content)
			}
			return onEvent(provider.StreamEvent{Content: "done", Finish: "stop"})
		},
	}

	_, csc, client, ctx2, cancel := startACPCatalogWithSandbox(t, rt, cat, fs, sandboxconfig.Engine{DataDir: root})
	defer cancel()
	client.permissionFn = func(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(acp.PermissionOptionId("allow_once")),
		}, nil
	}
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": agDef.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("read secret")},
	}); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.toolCallUpdates) != 1 || client.toolCallUpdates[0].Status == nil ||
		*client.toolCallUpdates[0].Status != acp.ToolCallStatusCompleted {
		t.Fatalf("updates = %+v", client.toolCallUpdates)
	}
}

func TestPromptPermissionRejectFailsTool(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("classified"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt := runtime.NewStore()
	cat, agDef := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sandbox.ProjectWorkspaceRoot(root, th.ProjectID), 0o755); err != nil {
		t.Fatal(err)
	}

	round := 0
	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			round++
			if round == 1 {
				return onEvent(provider.StreamEvent{
					Finish: "tool_calls",
					ToolCalls: []provider.ToolCall{{
						ID:        "call_deny",
						Name:      "read_file",
						Arguments: fmt.Sprintf(`{"path":%q}`, secret),
					}},
				})
			}
			if len(messages) < 3 || !strings.Contains(messages[2].Content, "permission rejected") {
				t.Fatalf("messages = %+v", messages)
			}
			return onEvent(provider.StreamEvent{Content: "blocked", Finish: "stop"})
		},
	}

	_, csc, client, ctx2, cancel := startACPCatalogWithSandbox(t, rt, cat, fs, sandboxconfig.Engine{DataDir: root})
	defer cancel()
	client.permissionFn = func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(acp.PermissionOptionId("reject_once")),
		}, nil
	}
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": agDef.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("read secret")},
	}); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.toolCallUpdates) != 1 || client.toolCallUpdates[0].Status == nil ||
		*client.toolCallUpdates[0].Status != acp.ToolCallStatusFailed {
		t.Fatalf("updates = %+v", client.toolCallUpdates)
	}
}

func TestPromptAskUserElicitation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rt := runtime.NewStore()
	cat, agDef := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sandbox.ProjectWorkspaceRoot(root, th.ProjectID), 0o755); err != nil {
		t.Fatal(err)
	}

	round := 0
	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			round++
			if round == 1 {
				return onEvent(provider.StreamEvent{
					Finish: "tool_calls",
					ToolCalls: []provider.ToolCall{{
						ID:   "call_ask",
						Name: "ask_user",
						Arguments: `{"questions":[{"id":"approach","question":"Which approach?","options":[{"label":"Safe"},{"label":"Fast"}]}]}`,
					}},
				})
			}
			if len(messages) < 3 || messages[2].Role != "tool" {
				t.Fatalf("messages = %+v", messages)
			}
			if !strings.Contains(messages[2].Content, `"answer":"Safe"`) {
				t.Fatalf("tool result = %s", messages[2].Content)
			}
			return onEvent(provider.StreamEvent{Content: "ok", Finish: "stop"})
		},
	}

	_, csc, client, ctx2, cancel := startACPCatalogWithSandbox(t, rt, cat, fs, sandboxconfig.Engine{DataDir: root})
	defer cancel()
	client.elicitationFn = func(_ context.Context, req acp.UnstableCreateElicitationRequest) (acp.UnstableCreateElicitationResponse, error) {
		if req.Form == nil {
			t.Fatalf("expected form elicitation")
		}
		resp := acp.NewUnstableCreateElicitationResponseAccept()
		resp.Accept.Content = map[string]any{"approach": "Safe"}
		return resp, nil
	}
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		Meta:            map[string]any{"elicitation": map[string]any{"form": map[string]any{}}},
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": agDef.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("clarify")},
	}); err != nil {
		t.Fatal(err)
	}
}
