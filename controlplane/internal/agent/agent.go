package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/gitrepo"
	"github.com/tryy3/agent-fabric/internal/integration"
	"github.com/tryy3/agent-fabric/internal/mcp/streamable"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	sandboxtools "github.com/tryy3/agent-fabric/internal/sandbox/tools"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/askuser"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
	filetools "github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/web"
	"github.com/tryy3/agent-fabric/internal/scrub"
)

type Agent struct {
	store           *runtime.Store
	catalog         *catalog.Store
	engine          engineconfig.Engine
	testStreamer    provider.ChatStreamer
	testEnvironment func(context.Context, sandbox.OpenOptions) (sandbox.Environment, error)

	mu         sync.Mutex
	conn       *acp.AgentSideConnection
	sessions   map[string]struct{}
	cancels    map[string]*context.CancelFunc
	grants     map[string][]sandbox.PathGrant
	cmdGrants  map[string][]string
	clientCaps acp.ClientCapabilities
	clientMeta map[string]any
	closed     bool
}

func New(
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine engineconfig.Engine,
) *Agent {
	return &Agent{
		store:     store,
		catalog:   catalogStore,
		engine:    engine,
		sessions:  make(map[string]struct{}),
		cancels:   make(map[string]*context.CancelFunc),
		grants:    make(map[string][]sandbox.PathGrant),
		cmdGrants: make(map[string][]string),
	}
}

func (a *Agent) SetTestStreamer(s provider.ChatStreamer) {
	a.testStreamer = s
}

// SetTestEnvironment replaces sandbox.Open during prompt tests.
func (a *Agent) SetTestEnvironment(open func(context.Context, sandbox.OpenOptions) (sandbox.Environment, error)) {
	a.testEnvironment = open
}

func (a *Agent) streamerFor(pin runtime.SessionPin, sessionID string) (provider.ChatStreamer, error) {
	if a.testStreamer != nil {
		return a.testStreamer, nil
	}
	return provider.NewStreamer(pin.ConnectionType, pin.BaseURL, pin.APIKey, provider.StreamerOpts{
		SessionID: sessionID,
		APIModes:  pin.WireModes,
	})
}

func streamOptionsFromPin(pin runtime.SessionPin) provider.StreamChatOptions {
	inf := pin.Inference
	return provider.StreamChatOptions{
		Instructions:      pin.EffectiveInstructions,
		Temperature:       inf.Temperature,
		TopP:              inf.TopP,
		MaxTokens:         inf.MaxTokens,
		ReasoningEffort:   inf.ReasoningEffort,
		TopK:              inf.TopK,
		MinP:              inf.MinP,
		RepetitionPenalty: inf.RepetitionPenalty,
		PresencePenalty:   inf.PresencePenalty,
		FrequencyPenalty:  inf.FrequencyPenalty,
		EnableThinking:    inf.EnableThinking,
		ThinkingType:      inf.ThinkingType,
		SamplerExtras:     catalog.SupportsSamplerExtras(pin.ConnectionType),
		UnslothExtras:     pin.ConnectionType == catalog.TypeUnslothStudio,
		BergetExtras:      pin.ConnectionType == catalog.TypeBergetAI,
	}
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
	if err := a.applyEffectiveInstructions(ctx, &pin, threadID); err != nil {
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
	slog.Info("session/new", "session", id, "agent", pin.AssistantID, "model", pin.CurrentModel, "thread", threadID)
	resp := acp.NewSessionResponse{
		SessionId:     acp.SessionId(id),
		ConfigOptions: modelConfigOptions(pin),
	}
	meta := map[string]any{}
	if threadID != "" {
		meta["threadId"] = threadID
	}
	if text := strings.TrimSpace(pin.EffectiveInstructions); text != "" {
		meta["agentFabric"] = map[string]any{
			"kind": "sent",
			"text": text,
		}
	}
	if len(meta) > 0 {
		resp.Meta = meta
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
		if !m.Active {
			continue
		}
		if m.Role == "assistant" && m.Status != string(catalog.AttemptStatusCompleted) {
			continue
		}
		history = append(history, runtime.Message{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: reasoningFromParts(m.Parts),
		})
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
	if err := a.catalog.PinThreadAssistant(ctx, threadID, pin.AssistantID); err != nil {
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
	assistantID, err := metaAssistantID(meta)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	ag, err := a.catalog.GetAssistant(ctx, assistantID)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	if !ag.IsComplete() {
		return runtime.SessionPin{}, fmt.Errorf("agent %q has no provider", ag.ID)
	}
	p, err := a.catalog.GetInferenceConnection(ctx, *ag.InferenceConnectionID)
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
	inf, err := catalog.InferenceFromSettings(ag.Settings)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	perms, err := catalog.PermissionsFromSettings(ag.Settings)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	gatePin, err := a.gatePin(ctx, perms)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	webPin, err := integration.ResolveWebPin(ctx, a.catalog, ag.Settings)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	pin := runtime.SessionPin{
		AssistantID:             ag.ID,
		AssistantName:           ag.Name,
		AssistantVersion:        ag.Version,
		InferenceConnectionID:   p.ID,
		InferenceConnectionName: p.Name,
		ConnectionType:          p.Type,
		BaseURL:                 p.BaseURL,
		APIKey:                  p.APIKey,
		Models:                  models,
		CurrentModel:            *ag.DefaultModel,
		Prices:                  a.catalog.ModelPrices(p.Type, p.BaseURL, p.Models),
		WireModes:               a.catalog.ModelWireModes(p.Type, p.BaseURL, p.Models),
		PermissionMode:          perms.Mode,
		Gate:                    gatePin,
		Inference: runtime.Inference{
			Temperature:       inf.Temperature,
			TopP:              inf.TopP,
			MaxTokens:         inf.MaxTokens,
			ReasoningEffort:   inf.ReasoningEffort,
			TopK:              inf.TopK,
			MinP:              inf.MinP,
			RepetitionPenalty: inf.RepetitionPenalty,
			PresencePenalty:   inf.PresencePenalty,
			FrequencyPenalty:  inf.FrequencyPenalty,
			EnableThinking:    inf.EnableThinking,
			ThinkingType:      inf.ThinkingType,
		},
	}
	if webPin.WebSearch != nil {
		pin.WebSearch = runtimeWebPin(webPin.WebSearch)
	}
	if webPin.FetchPage != nil {
		pin.FetchPage = runtimeWebPin(webPin.FetchPage)
	}
	return pin, nil
}

// applyEffectiveInstructions composes Platform + Assistant + Runtime Context
// instructions and substitutes {{variables}} using the bound thread's workspace
// and the session's current model. Runs after bindThread so model/workspace are
// final for the pin.
func (a *Agent) applyEffectiveInstructions(ctx context.Context, pin *runtime.SessionPin, threadID string) error {
	if a.catalog == nil || pin == nil {
		return nil
	}
	ag, err := a.catalog.GetAssistant(ctx, pin.AssistantID)
	if err != nil {
		return err
	}
	planeSettings, err := a.catalog.GetPlaneSettings(ctx)
	if err != nil {
		return err
	}
	vars := a.instructionVars(ctx, threadID, pin.CurrentModel)
	composed := catalog.ComposeEffectiveInstructions(
		planeSettings.PlatformInstructions,
		ag.Instructions,
		planeSettings.RuntimeContext,
	)
	pin.EffectiveInstructions = catalog.ApplyInstructionVars(composed, vars)
	return nil
}

func (a *Agent) instructionVars(ctx context.Context, threadID, modelID string) catalog.InstructionVars {
	workspaceRoot := catalog.DefaultProjectRoot
	if threadID != "" {
		th, err := a.catalog.GetThread(ctx, threadID)
		if err == nil {
			resolved, resolveErr := a.catalog.ResolveEnvironment(ctx, th.ProjectID)
			if resolveErr == nil && strings.TrimSpace(resolved.ProjectRoot) != "" {
				workspaceRoot = resolved.ProjectRoot
			}
		}
	}
	return catalog.NewInstructionVars(workspaceRoot, modelID)
}

func runtimeWebPin(p *integration.PinnedIntegration) *runtime.WebIntegrationPin {
	if p == nil {
		return nil
	}
	secrets := map[string]string{}
	for k, v := range p.Secrets {
		secrets[k] = v
	}
	return &runtime.WebIntegrationPin{
		ID:       p.ID,
		Name:     p.Name,
		Kind:     p.Kind,
		Endpoint: p.Endpoint,
		Mode:     p.Mode,
		Secrets:  secrets,
		Config:   append([]byte(nil), p.Config...),
	}
}

func metaAssistantID(meta map[string]any) (string, error) {
	if meta == nil {
		return "", fmt.Errorf("assistantId is required")
	}
	v, ok := meta["assistantId"]
	if !ok {
		return "", fmt.Errorf("assistantId is required")
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("assistantId is required")
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

func metaRetryLatest(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	v, ok := meta["retryLatest"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
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
	retryLatest := metaRetryLatest(params.Meta)
	var retryTarget catalog.RetryTarget
	var retryCommitted bool
	if retryLatest {
		if sess.ThreadID == "" {
			err := fmt.Errorf("retryLatest requires a bound thread")
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
		if a.catalog == nil {
			err := fmt.Errorf("retryLatest requires catalog")
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
		target, prepErr := a.catalog.LatestRetryTarget(ctx, sess.ThreadID)
		if prepErr != nil {
			slog.Error("session/prompt failed", "session", sid, "err", prepErr)
			return acp.PromptResponse{}, prepErr
		}
		priorMsgs, ok := a.store.Messages(sid)
		if !ok {
			err := fmt.Errorf("session %s not found", sid)
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
		active, listErr := a.catalog.ActiveMessages(ctx, sess.ThreadID)
		if listErr != nil {
			slog.Error("session/prompt failed", "session", sid, "err", listErr)
			return acp.PromptResponse{}, listErr
		}
		// Drop the last active assistant from model/runtime context for this stream;
		// DB supersede happens only after a successful attempt commit.
		history := make([]runtime.Message, 0, len(active))
		for _, m := range active {
			if m.ID == target.AssistantMessageID {
				continue
			}
			if m.Role == "assistant" && m.Status != string(catalog.AttemptStatusCompleted) {
				continue
			}
			history = append(history, runtime.Message{
				Role:             m.Role,
				Content:          m.Content,
				ReasoningContent: reasoningFromParts(m.Parts),
			})
		}
		if err := a.store.ReplaceMessages(sid, history); err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
		defer func() {
			if retryCommitted {
				return
			}
			if restoreErr := a.store.ReplaceMessages(sid, priorMsgs); restoreErr != nil {
				slog.Error("session/prompt restore after failed retry", "session", sid, "err", restoreErr)
			}
		}()
		retryTarget = target
		text = target.UserText
		slog.Info("session/prompt retryLatest",
			"session", sid,
			"user_message", target.UserMessageID,
			"pending_supersede_assistant", target.AssistantMessageID,
		)
	}
	slog.Info("session/prompt start",
		"session", sid,
		"user_chars", len(text),
		"user_preview", preview(text, 80),
		"retry_latest", retryLatest,
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
	if retryLatest {
		// Active history already includes the last user message after supersede.
		msgs = existing
	} else if bound {
		msgs = append(append([]runtime.Message{}, existing...), userMsg)
	} else {
		msgs = existing
	}

	var env sandbox.Environment
	var openOpts sandbox.OpenOptions
	streamOptions := streamOptionsFromPin(sess.Pin)
	var registry *sandbox.Registry
	open := sandbox.Open
	if a.testEnvironment != nil {
		open = a.testEnvironment
	}
	if a.catalog != nil {
		opts, openErr := a.promptExecutionOptions(promptCtx, sess)
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
		registry, streamOptions.Tools, err = sandboxTools(env, a.webRegistry(promptCtx, sess))
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
	costs := newCostTracker(sess.Pin.Prices)
	var streamRounds int
	streamStart := time.Now()
	var ttftMs int64
	gotTTFT := false
	maxRounds := 1
	if len(streamOptions.Tools) > 0 {
		maxRounds = 8
	}
	finalRound := false
	scrubPipe := scrub.Default(scrub.NewBodyScrubberFromEnv())
	// A session is tainted once it has read web content, in this turn or an
	// earlier one.
	tainted := historyTainted(msgs)
	scorerRound := 0
	toolGate := a.gateFor(sess.Pin, sid, func(hop provider.HopCapture) {
		if !bound || a.catalog == nil {
			return
		}
		meta := map[string]any{"hop": "gate_scorer"}
		for k, v := range hop.Meta {
			meta[k] = v
		}
		if _, capErr := a.catalog.InsertLLMHopCapture(promptCtx, catalog.InsertLLMHopCaptureParams{
			ThreadID:    sess.ThreadID,
			SessionID:   sid,
			RoundIndex:  scorerRound,
			Method:      hop.Method,
			URL:         hop.URL,
			StatusCode:  hop.StatusCode,
			ReqHeaders:  hop.ReqHeaders,
			RespHeaders: hop.RespHeaders,
			ReqBody:     string(hop.ReqBody),
			RespBody:    string(hop.RespBody),
			Meta:        meta,
			Pipeline:    scrubPipe,
		}); capErr != nil {
			slog.Error("gate scorer capture insert failed", "session", sid, "err", capErr)
		}
	})
	var turnHandles catalog.TurnHandles
	turnBegun := false
	baseAssistant := catalog.AssistantTurn{
		Model:        sess.Pin.CurrentModel,
		ProviderID:   sess.Pin.InferenceConnectionID,
		ProviderName: sess.Pin.InferenceConnectionName,
	}
	checkpointParts := func() {
		if !bound || a.catalog == nil || !turnBegun {
			return
		}
		parts := append([]catalog.MessagePart(nil), orderedParts...)
		if thoughtSeg.Len() > 0 {
			parts = append(parts, catalog.MessagePart{Type: "thought", Text: thoughtSeg.String()})
		}
		if content.Len() > 0 {
			hasMsg := false
			for _, p := range parts {
				if p.Type == "message" {
					hasMsg = true
					break
				}
			}
			if !hasMsg {
				parts = append(parts, catalog.MessagePart{Type: "message", Text: content.String()})
			}
		}
		if err := a.catalog.CheckpointAssistantParts(context.Background(), sess.ThreadID, turnHandles.AssistantMessageID, content.String(), parts); err != nil {
			slog.Error("checkpoint assistant parts failed", "session", sid, "err", err)
		}
	}
	finalizeBound := func(status catalog.AttemptStatus, stopReason string, activate bool, failureText string) error {
		if !bound || a.catalog == nil || !turnBegun {
			return nil
		}
		flushThought()
		parts := append([]catalog.MessagePart(nil), orderedParts...)
		contentText := content.String()
		if contentText != "" {
			hasMsg := false
			for _, p := range parts {
				if p.Type == "message" {
					hasMsg = true
					break
				}
			}
			if !hasMsg {
				parts = append(parts, catalog.MessagePart{Type: "message", Text: contentText})
			}
		}
		if failureText != "" {
			parts = append(parts, catalog.MessagePart{
				Type:   "error",
				Text:   failureText,
				Status: "failed",
			})
		}
		turn := baseAssistant
		turn.Content = contentText
		turn.StopReason = stopReason
		turn.Parts = parts
		turn.CaptureSessionID = sid
		return a.catalog.FinalizeAssistantAttempt(context.Background(), sess.ThreadID, turnHandles.AssistantMessageID, status, turn, activate)
	}
	if bound && a.catalog != nil {
		var beginErr error
		if retryLatest {
			turnHandles, beginErr = a.catalog.BeginAssistantAttempt(promptCtx, sess.ThreadID, retryTarget.UserMessageID, baseAssistant)
		} else {
			turnHandles, beginErr = a.catalog.BeginTurn(promptCtx, sess.ThreadID, text, baseAssistant)
		}
		if beginErr != nil {
			slog.Error("session/prompt failed", "session", sid, "err", beginErr)
			return acp.PromptResponse{}, beginErr
		}
		turnBegun = true
	}
	handlePromptErr := func(err error) (acp.PromptResponse, error) {
		if errors.Is(err, context.Canceled) {
			activate := !retryLatest
			if finErr := finalizeBound(catalog.AttemptStatusCancelled, string(acp.StopReasonCancelled), activate, ""); finErr != nil {
				slog.Error("finalize cancelled attempt failed", "session", sid, "err", finErr)
			}
			assistantMsg := runtime.Message{
				Role:             "assistant",
				Content:          content.String(),
				ReasoningContent: reasoningFromParts(orderedParts),
			}
			if bound && !retryLatest {
				if appErr := a.store.Append(sid, userMsg); appErr != nil {
					slog.Error("session/prompt runtime append failed after cancel", "session", sid, "err", appErr)
				}
			}
			if bound {
				if appErr := a.store.Append(sid, assistantMsg); appErr != nil {
					slog.Error("session/prompt runtime append failed after cancel", "session", sid, "err", appErr)
				}
			}
			slog.Info("session/prompt cancelled", "session", sid, "deltas", deltas, "assistant_chars", content.Len())
			return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
		}
		activate := !retryLatest
		failureText := formatInferenceFailure(baseAssistant.ProviderName, err)
		if finErr := finalizeBound(catalog.AttemptStatusFailed, "error", activate, failureText); finErr != nil {
			slog.Error("finalize failed attempt failed", "session", sid, "err", finErr)
		}
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "deltas", deltas, "err", err)
		return acp.PromptResponse{}, errors.New(failureText)
	}
	flushBufferedRound := func(roundContent *strings.Builder) {
		roundText := roundContent.String()
		if roundText == "" {
			return
		}
		content.WriteString(roundText)
		roundContent.Reset()
		if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update:    acp.UpdateAgentMessageText(roundText),
		}); err != nil {
			slog.Error("session update after stream failure", "session", sid, "err", err)
		}
	}
	for range maxRounds {
		var roundContent strings.Builder
		roundToolCalls := make([]provider.ToolCall, 0)
		var roundUsage *provider.Usage
		// roundIdx is this round's index in the turn's per-round usage, or nil when
		// the provider reported none; stored on its tool calls so clients can
		// place the round's cost in the transcript.
		var roundIdx *int
		lastFinish = ""
		streamRounds++
		roundIndex := streamRounds - 1
		scorerRound = roundIndex
		streamOptions.OnCapture = func(hop provider.HopCapture) {
			if !bound || a.catalog == nil {
				return
			}
			if _, capErr := a.catalog.InsertLLMHopCapture(promptCtx, catalog.InsertLLMHopCaptureParams{
				ThreadID:    sess.ThreadID,
				SessionID:   sid,
				RoundIndex:  roundIndex,
				Method:      hop.Method,
				URL:         hop.URL,
				StatusCode:  hop.StatusCode,
				ReqHeaders:  hop.ReqHeaders,
				RespHeaders: hop.RespHeaders,
				ReqBody:     string(hop.ReqBody),
				RespBody:    string(hop.RespBody),
				Meta:        hop.Meta,
				Pipeline:    scrubPipe,
			}); capErr != nil {
				slog.Error("hop capture insert failed", "session", sid, "round", roundIndex, "err", capErr)
			}
		}
		sentPart := sentMessagePart(streamOptions.Instructions)
		if sentPart.Text != "" && !hasSentPart(orderedParts) {
			orderedParts = append(orderedParts, sentPart)
			checkpointParts()
			if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update: acp.SessionUpdate{
					SessionInfoUpdate: &acp.SessionSessionInfoUpdate{
						SessionUpdate: "session_info_update",
						Meta: map[string]any{
							"agentFabric": map[string]any{
								"kind": "sent",
								"text": sentPart.Text,
							},
						},
					},
				},
			}); err != nil {
				slog.Error("session sent update failed", "session", sid, "err", err)
			}
		}
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
			flushBufferedRound(&roundContent)
			return handlePromptErr(err)
		}
		if roundUsage != nil {
			addUsage(&usage, *roundUsage)
			hasUsage = true
			rec := costs.addRound(sess.Pin.CurrentModel, *roundUsage)
			roundIdx = &rec.Round
			if len(roundToolCalls) > 0 {
				// Another round follows: report cost now rather than at turn end.
				if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
					SessionId: params.SessionId,
					Update: acp.SessionUpdate{UsageUpdate: &acp.SessionUsageUpdate{
						SessionUpdate: "usage_update",
						Used:          derefInt(roundUsage.TotalTokens),
						Size:          0,
						Meta:          costs.roundMeta(rec),
					}},
				}); err != nil {
					return handlePromptErr(err)
				}
			}
		}
		if len(roundToolCalls) == 0 {
			roundText := roundContent.String()
			if roundText != "" {
				content.WriteString(roundText)
				if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
					SessionId: params.SessionId,
					Update:    acp.UpdateAgentMessageText(roundText),
				}); err != nil {
					return handlePromptErr(err)
				}
			}
			flushThought()
			if content.Len() > 0 {
				orderedParts = append(orderedParts, catalog.MessagePart{Type: "message", Text: content.String()})
			}
			checkpointParts()
			finalRound = true
			break
		}

		roundReasoning := thoughtSeg.String()
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
			Role:             "assistant",
			Content:          roundContent.String(),
			ReasoningContent: roundReasoning,
			ToolCalls:        assistantToolCalls,
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
				return handlePromptErr(err)
			}

			gateTr := gateTrace{Mode: a.modeFor(sess.Pin), Tainted: tainted}
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
					toolGate,
					a.policyFor(sess.Pin, tainted),
					gateContext{Intent: text, Tainted: tainted},
					&gateTr,
				)
			}()
			if callErr != nil && errors.Is(callErr, context.Canceled) {
				return handlePromptErr(callErr)
			}
			result, failed := normalizeToolResult(result, callErr)
			status := acp.ToolCallStatusCompleted
			if failed {
				status = acp.ToolCallStatusFailed
			}
			if !failed && (filetools.IsMutating(call.Name) || call.Name == command.Name) {
				filesMutated = true
			}
			gateMeta := gateTr.meta()
			update := acp.UpdateToolCall(
				acp.ToolCallId(call.ID),
				acp.WithUpdateStatus(status),
				acp.WithUpdateRawOutput(jsonValueOrString(result)),
				acp.WithUpdateContent([]acp.ToolCallContent{
					acp.ToolContent(acp.TextBlock(result)),
				}),
			)
			if gateMeta != nil {
				// Shown to the user and stored with the transcript; the model
				// only ever receives the result text below.
				update.ToolCallUpdate.Meta = map[string]any{"gate": gateMeta}
			}
			if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update:    update,
			}); err != nil {
				return handlePromptErr(err)
			}
			orderedParts = append(orderedParts, catalog.MessagePart{
				Type:       "tool_call",
				Round:      roundIdx,
				ToolCallID: call.ID,
				Name:       call.Name,
				Title:      title,
				Input:      call.Arguments,
				Output:     result,
				Status:     string(status),
				Gate:       gateMeta,
			})
			checkpointParts()
			msgs = append(msgs, runtime.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
			if !failed && readsWebContent(call.Name) {
				tainted = true
			}
		}
	}
	if !finalRound {
		return handlePromptErr(fmt.Errorf("tool round limit exceeded"))
	}
	if content.Len() == 0 {
		return handlePromptErr(fmt.Errorf("empty assistant stream"))
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
			Meta:          usageMeta(*u, stopReason, costs),
		}},
	}); err != nil {
		return handlePromptErr(err)
	}

	contentText := content.String()
	flushThought()
	// Rebuild message part into turnParts shape with usage.
	finalParts := turnParts(filterNonUsageParts(orderedParts), contentText, *u, costs)
	orderedParts = finalParts
	assistantMsg := runtime.Message{
		Role:             "assistant",
		Content:          contentText,
		ReasoningContent: reasoningFromParts(orderedParts),
	}
	if bound {
		if retryLatest {
			if err := a.catalog.SupersedeAssistantAttempt(ctx, sess.ThreadID, retryTarget); err != nil {
				slog.Error("session/prompt failed", "session", sid, "err", err)
				return acp.PromptResponse{}, err
			}
		}
		baseAssistant.Content = contentText
		baseAssistant.StopReason = string(stopReason)
		baseAssistant.Parts = finalParts
		baseAssistant.CaptureSessionID = sid
		if err := a.catalog.FinalizeAssistantAttempt(ctx, sess.ThreadID, turnHandles.AssistantMessageID, catalog.AttemptStatusCompleted, baseAssistant, true); err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
		}
		turnBegun = false // already finalized
		if retryLatest {
			retryCommitted = true
		}
		if filesMutated {
			detail, getErr := a.catalog.GetThread(ctx, sess.ThreadID)
			if getErr != nil {
				slog.Error("session/prompt get thread after commit", "session", sid, "err", getErr)
			} else {
				a.autoCommitWorkspace(promptCtx, env, detail.Thread, text)
			}
		}
		if retryLatest {
			if err := a.store.Append(sid, assistantMsg); err != nil {
				slog.Error("session/prompt runtime append failed after commit", "session", sid, "err", err)
			}
		} else if err := a.store.Append(sid, userMsg); err != nil {
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

func filterNonUsageParts(parts []catalog.MessagePart) []catalog.MessagePart {
	out := make([]catalog.MessagePart, 0, len(parts))
	for _, p := range parts {
		if p.Type == "usage" {
			continue
		}
		if p.Type == "message" {
			// turnParts will re-add the final message text.
			continue
		}
		out = append(out, p)
	}
	return out
}

// formatInferenceFailure builds a transparent operator-facing error that keeps
// the upstream provider/plane detail and adds a short failure context prefix.
func formatInferenceFailure(providerName string, err error) string {
	raw := ""
	if err != nil {
		raw = strings.TrimSpace(err.Error())
	}
	if raw == "" {
		raw = "unknown error"
	}
	raw = scrub.SecretsInText(raw)
	if providerName != "" {
		return fmt.Sprintf("Inference failed (%s): %s", providerName, raw)
	}
	return fmt.Sprintf("Inference failed: %s", raw)
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

// reasoningFromParts concatenates thought parts for LLM history replay.
func reasoningFromParts(parts []catalog.MessagePart) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "thought" && p.Text != "" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func addUsage(total *provider.Usage, round provider.Usage) {
	addOptionalInt(&total.PromptTokens, round.PromptTokens)
	addOptionalInt(&total.CompletionTokens, round.CompletionTokens)
	addOptionalInt(&total.TotalTokens, round.TotalTokens)
	addOptionalInt(&total.CachedTokens, round.CachedTokens)
	addOptionalInt(&total.CacheWriteTokens, round.CacheWriteTokens)
	addOptionalInt(&total.ReasoningTokens, round.ReasoningTokens)
	addOptionalFloat64(&total.ReportedCostUSD, round.ReportedCostUSD)
	addOptionalFloat64(&total.PromptMs, round.PromptMs)
	addOptionalFloat64(&total.PredictedMs, round.PredictedMs)
	addOptionalFloat64(&total.Co2Grams, round.Co2Grams)
	addOptionalFloat64(&total.GpuEnergyJoules, round.GpuEnergyJoules)
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

func usageMeta(u provider.Usage, stopReason acp.StopReason, costs *costTracker) map[string]any {
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
	if u.CachedTokens != nil {
		m["cachedTokens"] = *u.CachedTokens
	}
	if u.CacheWriteTokens != nil {
		m["cacheWriteTokens"] = *u.CacheWriteTokens
	}
	if u.ReasoningTokens != nil {
		m["reasoningTokens"] = *u.ReasoningTokens
	}
	if costs != nil {
		if t := costs.totalCost(); t != nil {
			m["cost"] = costMeta(t)
		}
		if r := costs.reportedTotal(); r != nil {
			m["reportedCostUsd"] = *r
		}
		if len(costs.rounds) > 1 {
			m["rounds"] = costs.roundsMeta()
		}
	}
	if u.Co2Grams != nil {
		m["co2Grams"] = *u.Co2Grams
	}
	if u.GpuEnergyJoules != nil {
		m["gpuEnergyJoules"] = *u.GpuEnergyJoules
	}
	for k, v := range u.Extras {
		if _, exists := m[k]; exists {
			continue
		}
		m[k] = v
	}
	return m
}

func turnParts(
	activity []catalog.MessagePart,
	message string,
	u provider.Usage,
	costs *costTracker,
) []catalog.MessagePart {
	parts := make([]catalog.MessagePart, 0, len(activity)+2)
	parts = append(parts, activity...)
	parts = append(parts, catalog.MessagePart{Type: "message", Text: message})
	deltas := u.Deltas
	usagePart := catalog.MessagePart{
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
		Co2Grams:           u.Co2Grams,
		GpuEnergyJoules:    u.GpuEnergyJoules,
		Deltas:             &deltas,
		CachedTokens:       u.CachedTokens,
		CacheWriteTokens:   u.CacheWriteTokens,
		ReasoningTokens:    u.ReasoningTokens,
	}
	if costs != nil {
		usagePart.Cost = costs.totalCost()
		usagePart.ReportedCostUSD = costs.reportedTotal()
		if len(costs.rounds) > 1 {
			usagePart.Rounds = costs.rounds
		}
	}
	parts = append(parts, usagePart)
	return parts
}

func sandboxTools(env sandbox.Environment, webRunner *web.Runner) (*sandbox.Registry, []provider.ToolDefinition, error) {
	registry := sandboxtools.DefaultRegistry()
	if webRunner != nil {
		webRunner.Register(registry)
	}
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

func (a *Agent) webRegistry(ctx context.Context, sess runtime.Session) *web.Runner {
	if sess.Pin.WebSearch == nil && sess.Pin.FetchPage == nil {
		return nil
	}
	reg := &integration.Registry{
		MCPFactory: func(endpoint string, headers http.Header) integration.MCPClient {
			return &mcpClientAdapter{Client: &streamable.Client{Endpoint: endpoint, Headers: headers}}
		},
	}
	if a.catalog != nil && sess.ThreadID != "" {
		threadID := sess.ThreadID
		sessionID := sess.ID
		reg.Capture = func(ctx context.Context, hopKind, method, url string, status int, reqBody, respBody string, meta map[string]any) {
			_, _ = a.catalog.InsertHTTPHopCapture(ctx, catalog.InsertHTTPHopCaptureParams{
				ThreadID:   threadID,
				SessionID:  sessionID,
				RoundIndex: 0,
				HopKind:    hopKind,
				Method:     method,
				URL:        url,
				StatusCode: status,
				ReqBody:    reqBody,
				RespBody:   respBody,
				Meta:       meta,
				Pipeline:   scrub.Pipeline{Headers: scrub.DefaultHeaders{}, Body: scrub.Identity{}},
			})
		}
	}
	return &web.Runner{
		Registry: reg,
		Pin:      integrationWebPin(sess.Pin),
	}
}

func integrationWebPin(pin runtime.SessionPin) integration.WebPin {
	var out integration.WebPin
	if pin.WebSearch != nil {
		p := pinnedFromRuntime(pin.WebSearch)
		out.WebSearch = &p
	}
	if pin.FetchPage != nil {
		p := pinnedFromRuntime(pin.FetchPage)
		out.FetchPage = &p
	}
	return out
}

func pinnedFromRuntime(p *runtime.WebIntegrationPin) integration.PinnedIntegration {
	secrets := catalog.ToolIntegrationSecrets{}
	for k, v := range p.Secrets {
		secrets[k] = v
	}
	return integration.PinnedIntegration{
		ID:       p.ID,
		Name:     p.Name,
		Kind:     p.Kind,
		Endpoint: p.Endpoint,
		Mode:     p.Mode,
		Secrets:  secrets,
		Config:   append(json.RawMessage(nil), p.Config...),
	}
}

type mcpClientAdapter struct {
	*streamable.Client
}

func (a *mcpClientAdapter) ListTools(ctx context.Context) ([]integration.MCPTool, error) {
	tools, err := a.Client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]integration.MCPTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, integration.MCPTool{Name: t.Name, Description: t.Description})
	}
	return out, nil
}

func (a *Agent) promptExecutionOptions(ctx context.Context, sess runtime.Session) (sandbox.OpenOptions, error) {
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
	engine engineconfig.Engine,
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
	return catalog.AttachExecutionOptions(resolved, project.ID, engine.Docker.Runtime, engine.Docker.BinPath)
}

func toolPresentation(name string) (title string, kind acp.ToolKind) {
	switch name {
	case "read_file":
		return "Read file", acp.ToolKindRead
	case "write_file":
		return "Write file", acp.ToolKindEdit
	case "list_files":
		return "List files", acp.ToolKindSearch
	case "search_text":
		return "Search text", acp.ToolKindSearch
	case "apply_patch":
		return "Apply patch", acp.ToolKindEdit
	case "append_file":
		return "Append to file", acp.ToolKindEdit
	case "create_directory":
		return "Create directory", acp.ToolKindEdit
	case "move_path":
		return "Move path", acp.ToolKindMove
	case "delete_path":
		return "Delete path", acp.ToolKindDelete
	case command.Name:
		return "Run command", acp.ToolKindExecute
	case askuser.Name:
		return "Ask user", acp.ToolKindOther
	case web.SearchName:
		return "Web search", acp.ToolKindSearch
	case web.FetchName:
		return "Fetch page", acp.ToolKindFetch
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
	delete(a.grants, id)
	delete(a.cmdGrants, id)
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
