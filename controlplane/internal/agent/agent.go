package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/gitrepo"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	sandboxtools "github.com/tryy3/agent-fabric/internal/sandbox/tools"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/askuser"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

type Agent struct {
	store           *runtime.Store
	catalog         *catalog.Store
	engine          sandboxconfig.Engine
	testStreamer    provider.ChatStreamer
	testEnvironment func(context.Context, sandbox.OpenOptions) (sandbox.Environment, error)
	gate            gate.Chain

	mu         sync.Mutex
	conn       *acp.AgentSideConnection
	sessions   map[string]struct{}
	cancels    map[string]*context.CancelFunc
	grants     map[string][]sandbox.PathGrant
	clientCaps acp.ClientCapabilities
	clientMeta map[string]any
	closed     bool
}

func New(
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine sandboxconfig.Engine,
) *Agent {
	return &Agent{
		store:    store,
		catalog:  catalogStore,
		engine:   engine,
		gate:     gate.DefaultChain(),
		sessions: make(map[string]struct{}),
		cancels:  make(map[string]*context.CancelFunc),
		grants:   make(map[string][]sandbox.PathGrant),
	}
}

func (a *Agent) SetTestStreamer(s provider.ChatStreamer) {
	a.testStreamer = s
}

// SetTestEnvironment replaces sandbox.Open during prompt tests.
func (a *Agent) SetTestEnvironment(open func(context.Context, sandbox.OpenOptions) (sandbox.Environment, error)) {
	a.testEnvironment = open
}

// SetGate replaces the tool gate chain (tests / custom evaluators).
func (a *Agent) SetGate(chain gate.Chain) {
	a.gate = chain
}

func (a *Agent) streamerFor(pin runtime.SessionPin, sessionID string) (provider.ChatStreamer, error) {
	if a.testStreamer != nil {
		return a.testStreamer, nil
	}
	return provider.NewStreamer(pin.ProviderType, pin.BaseURL, pin.APIKey, provider.StreamerOpts{
		SessionID: sessionID,
	})
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
	slog.Info("acp initialize", "protocol_version", params.ProtocolVersion)
	a.mu.Lock()
	a.clientCaps = params.ClientCapabilities
	a.clientMeta = params.Meta
	a.mu.Unlock()
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
		},
	}, nil
}

func (a *Agent) NewSession(ctx context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.mu.Lock()
	closed := a.closed
	a.mu.Unlock()
	if closed {
		return acp.NewSessionResponse{}, fmt.Errorf("connection closed")
	}

	pin, err := a.pinFromCatalog(ctx, params.Meta)
	if err != nil {
		slog.Error("session/new failed", "err", err)
		return acp.NewSessionResponse{}, err
	}
	threadID, history, persistDefaultModel, err := a.bindThread(ctx, params.Meta, &pin)
	if err != nil {
		slog.Error("session/new failed", "err", err)
		return acp.NewSessionResponse{}, err
	}
	id, err := a.store.CreateHydrated(pin, threadID, history)
	if err != nil {
		slog.Error("session/new failed", "err", err)
		return acp.NewSessionResponse{}, err
	}
	if err := a.commitNewSession(id); err != nil {
		slog.Error("session/new failed", "err", err)
		return acp.NewSessionResponse{}, err
	}
	if threadID != "" {
		if err := a.pinLiveThread(ctx, id, threadID, pin, persistDefaultModel); err != nil {
			slog.Error("session/new failed", "err", err)
			return acp.NewSessionResponse{}, err
		}
	}
	slog.Info("session/new", "session", id, "agent", pin.AgentID, "model", pin.CurrentModel, "thread", threadID)
	resp := acp.NewSessionResponse{
		SessionId:     acp.SessionId(id),
		ConfigOptions: modelConfigOptions(pin),
	}
	if threadID != "" {
		resp.Meta = map[string]any{"threadId": threadID}
	}
	return resp, nil
}

func (a *Agent) bindThread(ctx context.Context, meta map[string]any, pin *runtime.SessionPin) (string, []runtime.Message, bool, error) {
	threadID, err := metaThreadID(meta)
	if err != nil {
		return "", nil, false, err
	}
	if threadID == "" {
		return "", nil, false, nil
	}
	detail, err := a.catalog.GetThread(ctx, threadID)
	if err != nil {
		return "", nil, false, err
	}
	history := make([]runtime.Message, 0, len(detail.Messages))
	for _, m := range detail.Messages {
		history = append(history, runtime.Message{Role: m.Role, Content: m.Content})
	}
	persistDefaultModel := true
	if detail.CurrentModel != nil {
		persistDefaultModel = false
		for _, m := range pin.Models {
			if m.ID == *detail.CurrentModel {
				pin.CurrentModel = *detail.CurrentModel
				break
			}
		}
	}
	return threadID, history, persistDefaultModel, nil
}

func (a *Agent) pinLiveThread(ctx context.Context, sessionID, threadID string, pin runtime.SessionPin, persistDefaultModel bool) error {
	if err := a.catalog.PinThreadAgent(ctx, threadID, pin.AgentID); err != nil {
		a.dropLiveSession(sessionID)
		return err
	}
	if persistDefaultModel {
		if err := a.catalog.SetThreadModel(ctx, threadID, pin.CurrentModel); err != nil {
			a.dropLiveSession(sessionID)
			return err
		}
	}
	return nil
}

func (a *Agent) dropLiveSession(id string) {
	a.store.Delete(id)
	a.mu.Lock()
	delete(a.sessions, id)
	a.mu.Unlock()
}

func (a *Agent) commitNewSession(id string) error {
	a.mu.Lock()
	closed := a.closed
	if !closed {
		a.sessions[id] = struct{}{}
	}
	a.mu.Unlock()
	if closed {
		a.store.Delete(id)
		return fmt.Errorf("connection closed")
	}
	return nil
}

func (a *Agent) pinFromCatalog(ctx context.Context, meta map[string]any) (runtime.SessionPin, error) {
	if a.catalog == nil {
		return runtime.SessionPin{}, fmt.Errorf("catalog not configured")
	}
	agentID, err := metaAgentID(meta)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	ag, err := a.catalog.GetAgent(ctx, agentID)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	if !ag.IsComplete() {
		return runtime.SessionPin{}, fmt.Errorf("agent %q has no provider", ag.ID)
	}
	p, err := a.catalog.GetProvider(ctx, *ag.ProviderID)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	if len(p.Models) == 0 {
		return runtime.SessionPin{}, fmt.Errorf("provider %q has no models", p.ID)
	}
	models := make([]runtime.ModelRef, 0, len(p.Models))
	foundDefault := false
	for _, m := range p.Models {
		models = append(models, runtime.ModelRef{ID: m.ID, Name: m.Name})
		if m.ID == *ag.DefaultModel {
			foundDefault = true
		}
	}
	if !foundDefault {
		return runtime.SessionPin{}, fmt.Errorf("default model %q not in provider cache", *ag.DefaultModel)
	}
	return runtime.SessionPin{
		AgentID:      ag.ID,
		AgentName:    ag.Name,
		AgentVersion: ag.Version,
		ProviderID:   p.ID,
		ProviderName: p.Name,
		ProviderType: p.Type,
		BaseURL:      p.BaseURL,
		APIKey:       p.APIKey,
		Models:       models,
		CurrentModel: *ag.DefaultModel,
	}, nil
}

func metaAgentID(meta map[string]any) (string, error) {
	if meta == nil {
		return "", fmt.Errorf("agentId is required")
	}
	v, ok := meta["agentId"]
	if !ok {
		return "", fmt.Errorf("agentId is required")
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("agentId is required")
	}
	return s, nil
}

func metaThreadID(meta map[string]any) (string, error) {
	if meta == nil {
		return "", nil
	}
	v, ok := meta["threadId"]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("threadId is invalid")
	}
	return s, nil
}

func (a *Agent) Authenticate(ctx context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *Agent) SetSessionConfigOption(ctx context.Context, params acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	if params.ValueId == nil {
		return acp.SetSessionConfigOptionResponse{}, fmt.Errorf("unsupported config option variant")
	}
	if params.ValueId.ConfigId != acp.SessionConfigId("model") {
		return acp.SetSessionConfigOptionResponse{}, fmt.Errorf("unknown config option %q", params.ValueId.ConfigId)
	}
	sid := string(params.ValueId.SessionId)
	model := string(params.ValueId.Value)
	sess, ok := a.store.Get(sid)
	if !ok {
		err := fmt.Errorf("session %s not found", sid)
		slog.Error("session/set_config_option failed", "session", sid, "err", err)
		return acp.SetSessionConfigOptionResponse{}, err
	}
	previous := sess.Pin.CurrentModel
	if err := a.store.SetCurrentModel(sid, model); err != nil {
		slog.Error("session/set_config_option failed", "session", sid, "err", err)
		return acp.SetSessionConfigOptionResponse{}, err
	}
	if sess.ThreadID != "" {
		if err := a.catalog.SetThreadModel(ctx, sess.ThreadID, model); err != nil {
			if restoreErr := a.store.SetCurrentModel(sid, previous); restoreErr != nil {
				slog.Error("session/set_config_option restore failed", "session", sid, "err", restoreErr)
			}
			slog.Error("session/set_config_option failed", "session", sid, "err", err)
			return acp.SetSessionConfigOptionResponse{}, err
		}
	}
	sess, ok = a.store.Get(sid)
	if !ok {
		err := fmt.Errorf("session %s not found", sid)
		slog.Error("session/set_config_option failed", "session", sid, "err", err)
		return acp.SetSessionConfigOptionResponse{}, err
	}
	slog.Info("session/set_config_option", "session", sid, "model", sess.Pin.CurrentModel)
	return acp.SetSessionConfigOptionResponse{
		ConfigOptions: modelConfigOptions(sess.Pin),
	}, nil
}

func (a *Agent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	sid := string(params.SessionId)
	sess, ok := a.store.Get(sid)
	if !ok {
		err := fmt.Errorf("session %s not found", sid)
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}
	conn := a.connection()
	if conn == nil {
		err := fmt.Errorf("agent connection not set")
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}
	streamer, err := a.streamerFor(sess.Pin, sid)
	if err != nil {
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}

	text := provider.PromptText(params.Prompt)
	slog.Info("session/prompt start",
		"session", sid,
		"user_chars", len(text),
		"user_preview", preview(text, 80),
	)
	userMsg := runtime.Message{Role: "user", Content: text}
	bound := sess.ThreadID != ""
	if !bound {
		if err := a.store.Append(sid, userMsg); err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
	}

	promptCtx, cancel := context.WithCancel(ctx)
	myCancel := &cancel
	a.mu.Lock()
	prev := a.cancels[sid]
	a.cancels[sid] = myCancel
	a.mu.Unlock()
	if prev != nil {
		slog.Info("session/prompt cancelling previous in-flight turn", "session", sid)
		(*prev)()
	}
	defer func() {
		cancel()
		a.mu.Lock()
		if current, ok := a.cancels[sid]; ok && current == myCancel {
			delete(a.cancels, sid)
		}
		a.mu.Unlock()
	}()

	existing, ok := a.store.Messages(sid)
	if !ok {
		err := fmt.Errorf("session %s not found", sid)
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}
	var msgs []runtime.Message
	if bound {
		msgs = append(append([]runtime.Message{}, existing...), userMsg)
	} else {
		msgs = existing
	}

	var env sandbox.Environment
	var openOpts sandbox.OpenOptions
	streamOptions := provider.StreamChatOptions{}
	var registry *sandbox.Registry
	open := sandbox.Open
	if a.testEnvironment != nil {
		open = a.testEnvironment
	}
	if a.catalog != nil {
		opts, openErr := a.promptSandboxOptions(promptCtx, sess)
		if openErr != nil {
			slog.Error("session/prompt failed", "session", sid, "err", openErr)
			return acp.PromptResponse{}, openErr
		}
		opts = mergeOpenPolicy(opts, a.sessionGrants(sid))
		openOpts = opts
		if opts.Kind != "" {
			env, err = open(promptCtx, opts)
			if err != nil {
				slog.Error("session/prompt failed", "session", sid, "err", err)
				return acp.PromptResponse{}, err
			}
			defer func() {
				if closeErr := env.Close(context.Background()); closeErr != nil {
					slog.Error("sandbox close failed", "session", sid, "err", closeErr)
				}
			}()
		}
		registry, streamOptions.Tools, err = sandboxTools(env)
		if err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
	}

	var thoughtSeg, content strings.Builder
	orderedParts := make([]catalog.MessagePart, 0)
	filesMutated := false
	flushThought := func() {
		if thoughtSeg.Len() == 0 {
			return
		}
		orderedParts = append(orderedParts, catalog.MessagePart{
			Type: "thought",
			Text: thoughtSeg.String(),
		})
		thoughtSeg.Reset()
	}
	var deltas int
	var lastFinish string
	var usage provider.Usage
	var hasUsage bool
	var streamRounds int
	streamStart := time.Now()
	var ttftMs int64
	gotTTFT := false
	maxRounds := 1
	if len(streamOptions.Tools) > 0 {
		maxRounds = 8
	}
	finalRound := false
	for range maxRounds {
		var roundContent strings.Builder
		roundToolCalls := make([]provider.ToolCall, 0)
		var roundUsage *provider.Usage
		lastFinish = ""
		streamRounds++
		err = streamer.StreamChat(promptCtx, sess.Pin.CurrentModel, msgs, streamOptions, func(ev provider.StreamEvent) error {
			if ev.Finish != "" {
				lastFinish = ev.Finish
			}
			if ev.Usage != nil {
				u := *ev.Usage
				roundUsage = &u
			}
			if len(ev.ToolCalls) > 0 {
				roundToolCalls = append(roundToolCalls, ev.ToolCalls...)
			}
			if ev.Thought != "" || ev.Content != "" {
				if !gotTTFT {
					ttftMs = time.Since(streamStart).Milliseconds()
					gotTTFT = true
				}
			}
			if ev.Thought != "" {
				thoughtSeg.WriteString(ev.Thought)
				if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
					SessionId: params.SessionId,
					Update:    acp.UpdateAgentThoughtText(ev.Thought),
				}); err != nil {
					return err
				}
			}
			if ev.Content != "" {
				deltas++
				// Buffer only; emit after the round if it is final (no tool_calls).
				roundContent.WriteString(ev.Content)
			}
			return nil
		})
		if err != nil {
			slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "deltas", deltas, "err", err)
			return acp.PromptResponse{}, err
		}
		if roundUsage != nil {
			addUsage(&usage, *roundUsage)
			hasUsage = true
		}
		if len(roundToolCalls) == 0 {
			roundText := roundContent.String()
			if roundText != "" {
				content.WriteString(roundText)
				if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
					SessionId: params.SessionId,
					Update:    acp.UpdateAgentMessageText(roundText),
				}); err != nil {
					return acp.PromptResponse{}, err
				}
			}
			finalRound = true
			break
		}

		flushThought()
		assistantToolCalls := make([]runtime.ToolCall, 0, len(roundToolCalls))
		for _, call := range roundToolCalls {
			assistantToolCalls = append(assistantToolCalls, runtime.ToolCall{
				ID:   call.ID,
				Type: "function",
				Function: runtime.ToolCallFunction{
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			})
		}
		msgs = append(msgs, runtime.Message{
			Role:      "assistant",
			Content:   roundContent.String(),
			ToolCalls: assistantToolCalls,
		})
		for _, call := range roundToolCalls {
			title, kind := toolPresentation(call.Name)
			if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update: acp.StartToolCall(
					acp.ToolCallId(call.ID),
					title,
					acp.WithStartStatus(acp.ToolCallStatusPending),
					acp.WithStartRawInput(jsonValueOrString(call.Arguments)),
					acp.WithStartKind(kind),
				),
			}); err != nil {
				return acp.PromptResponse{}, err
			}

			result, callErr := func() (string, error) {
				if call.Name == askuser.Name {
					return a.runAskUser(
						promptCtx,
						conn,
						params.SessionId,
						call.ID,
						json.RawMessage(call.Arguments),
					)
				}
				if registry == nil || env == nil {
					return "", fmt.Errorf("sandbox tools are unavailable")
				}
				return a.runGatedTool(
					promptCtx,
					conn,
					params.SessionId,
					call.ID,
					call.Name,
					json.RawMessage(call.Arguments),
					openOpts,
					env,
					registry,
					open,
					a.gate,
				)
			}()
			result, failed := normalizeToolResult(result, callErr)
			status := acp.ToolCallStatusCompleted
			if failed {
				status = acp.ToolCallStatusFailed
			}
			if !failed && call.Name == "write_file" {
				filesMutated = true
			}
			if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update: acp.UpdateToolCall(
					acp.ToolCallId(call.ID),
					acp.WithUpdateStatus(status),
					acp.WithUpdateRawOutput(jsonValueOrString(result)),
					acp.WithUpdateContent([]acp.ToolCallContent{
						acp.ToolContent(acp.TextBlock(result)),
					}),
				),
			}); err != nil {
				return acp.PromptResponse{}, err
			}
			orderedParts = append(orderedParts, catalog.MessagePart{
				Type:       "tool_call",
				ToolCallID: call.ID,
				Name:       call.Name,
				Title:      title,
				Input:      call.Arguments,
				Output:     result,
				Status:     string(status),
			})
			msgs = append(msgs, runtime.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}
	}
	if !finalRound {
		err := fmt.Errorf("tool round limit exceeded")
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "err", err)
		return acp.PromptResponse{}, err
	}
	if content.Len() == 0 {
		err := fmt.Errorf("empty assistant stream")
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "err", err)
		return acp.PromptResponse{}, err
	}
	stopReason := mapFinishReason(lastFinish)
	u := &usage
	if !hasUsage {
		u = &provider.Usage{
			ElapsedMs: ptrInt64(time.Since(streamStart).Milliseconds()),
		}
	}
	u.Deltas = deltas
	if gotTTFT {
		u.TTFTMs = ptrInt64(ttftMs)
	}
	if streamRounds > 1 {
		u.PromptPerSecond = nil
		u.PredictedPerSecond = nil
	}
	used := 0
	if u.TotalTokens != nil {
		used = *u.TotalTokens
	}
	if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
		SessionId: params.SessionId,
		Update: acp.SessionUpdate{UsageUpdate: &acp.SessionUsageUpdate{
			SessionUpdate: "usage_update",
			Used:          used,
			Size:          0,
			Meta:          usageMeta(*u, stopReason),
		}},
	}); err != nil {
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}

	contentText := content.String()
	flushThought()
	assistantMsg := runtime.Message{Role: "assistant", Content: contentText}
	if bound {
		committed, err := a.catalog.CommitTurn(ctx, sess.ThreadID, text, catalog.AssistantTurn{
			Content:      contentText,
			Model:        sess.Pin.CurrentModel,
			ProviderID:   sess.Pin.ProviderID,
			ProviderName: sess.Pin.ProviderName,
			StopReason:   string(stopReason),
			Parts:        turnParts(orderedParts, contentText, *u),
		})
		if err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
		if filesMutated {
			a.autoCommitWorkspace(promptCtx, env, committed, text)
		}
		if err := a.store.Append(sid, userMsg); err != nil {
			slog.Error("session/prompt runtime append failed after commit", "session", sid, "err", err)
		} else if err := a.store.Append(sid, assistantMsg); err != nil {
			slog.Error("session/prompt runtime append failed after commit", "session", sid, "err", err)
		}
	} else {
		if err := a.store.Append(sid, assistantMsg); err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
	}
	slog.Info("session/prompt complete",
		"session", sid,
		"history_msgs", len(msgs)+1,
		"deltas", deltas,
		"assistant_chars", content.Len(),
		"assistant_preview", preview(contentText, 80),
	)
	return acp.PromptResponse{StopReason: stopReason}, nil
}

func (a *Agent) autoCommitWorkspace(ctx context.Context, env sandbox.Environment, thread catalog.Thread, userPrompt string) {
	if env == nil {
		return
	}
	execu, ok := env.Exec()
	if !ok {
		return
	}
	fsys, _ := env.FS()
	if err := gitrepo.EnsureRepo(ctx, execu, fsys); err != nil {
		slog.Warn("git auto-commit skipped", "thread", thread.ID, "err", err)
		return
	}
	msg := gitrepo.AgentCommitMessage(thread.Title, thread.ID, userPrompt)
	if _, _, err := gitrepo.CommitIfDirty(ctx, execu, msg); err != nil {
		slog.Warn("git auto-commit failed", "thread", thread.ID, "err", err)
	}
}

func mapFinishReason(finish string) acp.StopReason {
	switch finish {
	case "length":
		return acp.StopReasonMaxTokens
	case "content_filter":
		return acp.StopReasonRefusal
	default:
		return acp.StopReasonEndTurn
	}
}

func addUsage(total *provider.Usage, round provider.Usage) {
	addOptionalInt(&total.PromptTokens, round.PromptTokens)
	addOptionalInt(&total.CompletionTokens, round.CompletionTokens)
	addOptionalInt(&total.TotalTokens, round.TotalTokens)
	addOptionalFloat64(&total.PromptMs, round.PromptMs)
	addOptionalFloat64(&total.PredictedMs, round.PredictedMs)
	addOptionalInt64(&total.ElapsedMs, round.ElapsedMs)
	total.PromptPerSecond = round.PromptPerSecond
	total.PredictedPerSecond = round.PredictedPerSecond
}

func addOptionalInt(total **int, value *int) {
	if value == nil {
		return
	}
	if *total == nil {
		sum := *value
		*total = &sum
		return
	}
	**total += *value
}

func addOptionalInt64(total **int64, value *int64) {
	if value == nil {
		return
	}
	if *total == nil {
		sum := *value
		*total = &sum
		return
	}
	**total += *value
}

func addOptionalFloat64(total **float64, value *float64) {
	if value == nil {
		return
	}
	if *total == nil {
		sum := *value
		*total = &sum
		return
	}
	**total += *value
}

func usageMeta(u provider.Usage, stopReason acp.StopReason) map[string]any {
	m := map[string]any{"stopReason": string(stopReason), "deltas": u.Deltas}
	if u.TTFTMs != nil {
		m["ttftMs"] = *u.TTFTMs
	}
	if u.ElapsedMs != nil {
		m["elapsedMs"] = *u.ElapsedMs
	}
	if u.PromptMs != nil {
		m["promptMs"] = *u.PromptMs
	}
	if u.PredictedMs != nil {
		m["predictedMs"] = *u.PredictedMs
	}
	if u.PromptPerSecond != nil {
		m["promptPerSecond"] = *u.PromptPerSecond
	}
	if u.PredictedPerSecond != nil {
		m["predictedPerSecond"] = *u.PredictedPerSecond
	}
	if u.PromptTokens != nil {
		m["promptTokens"] = *u.PromptTokens
	}
	if u.CompletionTokens != nil {
		m["completionTokens"] = *u.CompletionTokens
	}
	if u.TotalTokens != nil {
		m["totalTokens"] = *u.TotalTokens
	}
	return m
}

func turnParts(
	activity []catalog.MessagePart,
	message string,
	u provider.Usage,
) []catalog.MessagePart {
	parts := make([]catalog.MessagePart, 0, len(activity)+2)
	parts = append(parts, activity...)
	parts = append(parts, catalog.MessagePart{Type: "message", Text: message})
	deltas := u.Deltas
	parts = append(parts, catalog.MessagePart{
		Type:               "usage",
		PromptTokens:       u.PromptTokens,
		CompletionTokens:   u.CompletionTokens,
		TotalTokens:        u.TotalTokens,
		ContextUsed:        u.TotalTokens,
		PromptMs:           u.PromptMs,
		PredictedMs:        u.PredictedMs,
		TTFTMs:             u.TTFTMs,
		ElapsedMs:          u.ElapsedMs,
		PromptPerSecond:    u.PromptPerSecond,
		PredictedPerSecond: u.PredictedPerSecond,
		Deltas:             &deltas,
	})
	return parts
}

func sandboxTools(env sandbox.Environment) (*sandbox.Registry, []provider.ToolDefinition, error) {
	registry := sandboxtools.DefaultRegistry()
	var available []sandbox.Tool
	if env != nil {
		available = registry.Available(env)
	} else {
		for _, tool := range registry.All() {
			if tool.Requires == (sandbox.Capabilities{}) {
				available = append(available, tool)
			}
		}
	}
	definitions := make([]provider.ToolDefinition, 0, len(available))
	for _, tool := range available {
		def, err := provider.FunctionTool(tool.Name, tool.Description, tool.Parameters)
		if err != nil {
			return nil, nil, fmt.Errorf("encode tool %q parameters: %w", tool.Name, err)
		}
		definitions = append(definitions, def)
	}
	return registry, definitions, nil
}

func (a *Agent) promptSandboxOptions(ctx context.Context, sess runtime.Session) (sandbox.OpenOptions, error) {
	if a.catalog == nil {
		return sandbox.OpenOptions{}, nil
	}
	var project catalog.Project
	if sess.ThreadID != "" {
		thread, err := a.catalog.GetThread(ctx, sess.ThreadID)
		if err != nil {
			return sandbox.OpenOptions{}, err
		}
		project, err = a.catalog.GetProject(ctx, thread.ProjectID)
		if err != nil {
			return sandbox.OpenOptions{}, err
		}
	}
	return openPromptSandbox(ctx, a.catalog, a.engine, project)
}

func openPromptSandbox(
	ctx context.Context,
	store *catalog.Store,
	engine sandboxconfig.Engine,
	project catalog.Project,
) (sandbox.OpenOptions, error) {
	if strings.TrimSpace(project.ID) == "" {
		return sandbox.OpenOptions{}, nil
	}
	resolved, err := store.ResolveEnvironment(ctx, project.ID)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	if resolved.Resource == nil {
		if resolved.ResourceID != nil && strings.TrimSpace(*resolved.ResourceID) != "" {
			return sandbox.OpenOptions{}, fmt.Errorf("resource %q not found", strings.TrimSpace(*resolved.ResourceID))
		}
		return sandbox.OpenOptions{}, fmt.Errorf("project %q has no resource", project.ID)
	}
	return catalog.AttachSandboxOptions(resolved, project.ID, engine.Docker.Runtime, engine.Docker.BinPath)
}

func toolPresentation(name string) (string, acp.ToolKind) {
	switch name {
	case "read_file":
		return "Read file", acp.ToolKindRead
	case "write_file":
		return "Write file", acp.ToolKindEdit
	case askuser.Name:
		return "Ask user", acp.ToolKindOther
	default:
		return name, acp.ToolKindOther
	}
}

func normalizeToolResult(result string, err error) (string, bool) {
	if err != nil {
		encoded, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
		if marshalErr != nil {
			return `{"error":"failed to encode tool error"}`, true
		}
		return string(encoded), true
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(result), &envelope) == nil && envelope.Error != "" {
		return result, true
	}
	return result, false
}

// jsonValueOrString unmarshals s as JSON for ACP rawInput/rawOutput; falls back to the raw string.
func jsonValueOrString(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}

func ptrInt64(v int64) *int64 { return &v }

func (a *Agent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	sid := string(params.SessionId)
	a.mu.Lock()
	cf := a.cancels[sid]
	a.mu.Unlock()
	if cf != nil {
		slog.Info("session/cancel", "session", sid, "had_inflight", true)
		(*cf)()
	} else {
		slog.Info("session/cancel", "session", sid, "had_inflight", false)
	}
	return nil
}

func (a *Agent) CloseSession(ctx context.Context, params acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	id := string(params.SessionId)
	slog.Info("session/close", "session", id)
	invokeCancels(a.takeCancels([]string{id}))
	a.store.Delete(id)
	a.mu.Lock()
	delete(a.sessions, id)
	a.mu.Unlock()
	return acp.CloseSessionResponse{}, nil
}

func (a *Agent) CloseConnectionSessions() {
	a.mu.Lock()
	a.closed = true
	ids := make([]string, 0, len(a.sessions))
	for id := range a.sessions {
		ids = append(ids, id)
	}
	clear(a.sessions)
	cfs := a.takeCancelsLocked(ids)
	a.mu.Unlock()
	if len(ids) > 0 {
		slog.Info("connection cleanup", "sessions", len(ids), "cancelling", len(cfs))
	}
	invokeCancels(cfs)

	for _, id := range ids {
		a.store.Delete(id)
	}
}

func preview(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (a *Agent) takeCancels(ids []string) []context.CancelFunc {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.takeCancelsLocked(ids)
}

func (a *Agent) takeCancelsLocked(ids []string) []context.CancelFunc {
	out := make([]context.CancelFunc, 0, len(ids))
	for _, id := range ids {
		if cf, ok := a.cancels[id]; ok && cf != nil {
			out = append(out, *cf)
			delete(a.cancels, id)
		}
	}
	return out
}

func invokeCancels(cfs []context.CancelFunc) {
	for _, cf := range cfs {
		cf()
	}
}
