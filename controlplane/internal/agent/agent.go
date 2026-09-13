package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type Agent struct {
	store        *runtime.Store
	catalog      *catalog.Store
	testStreamer provider.ChatStreamer

	mu       sync.Mutex
	conn     *acp.AgentSideConnection
	sessions map[string]struct{}
	cancels  map[string]*context.CancelFunc
	closed   bool
}

func New(store *runtime.Store, catalogStore *catalog.Store) *Agent {
	return &Agent{
		store:    store,
		catalog:  catalogStore,
		sessions: make(map[string]struct{}),
		cancels:  make(map[string]*context.CancelFunc),
	}
}

func (a *Agent) SetTestStreamer(s provider.ChatStreamer) {
	a.testStreamer = s
}

func (a *Agent) streamerFor(pin runtime.SessionPin) (provider.ChatStreamer, error) {
	if a.testStreamer != nil {
		return a.testStreamer, nil
	}
	return provider.NewStreamer(pin.ProviderType, pin.BaseURL, pin.APIKey, nil)
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
	streamer, err := a.streamerFor(sess.Pin)
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

	var thought, content strings.Builder
	var deltas int
	var lastFinish string
	var lastUsage *provider.Usage
	streamStart := time.Now()
	var ttftMs int64
	gotTTFT := false
	err = streamer.StreamChat(promptCtx, sess.Pin.CurrentModel, msgs, func(ev provider.StreamEvent) error {
		if ev.Finish != "" {
			lastFinish = ev.Finish
		}
		if ev.Usage != nil {
			u := *ev.Usage
			lastUsage = &u
		}
		if ev.Thought != "" || ev.Content != "" {
			if !gotTTFT {
				ttftMs = time.Since(streamStart).Milliseconds()
				gotTTFT = true
			}
		}
		if ev.Thought != "" {
			thought.WriteString(ev.Thought)
			if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update:    acp.UpdateAgentThoughtText(ev.Thought),
			}); err != nil {
				return err
			}
		}
		if ev.Content != "" {
			deltas++
			content.WriteString(ev.Content)
			if err := conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update:    acp.UpdateAgentMessageText(ev.Content),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "deltas", deltas, "err", err)
		return acp.PromptResponse{}, err
	}
	if content.Len() == 0 {
		err := fmt.Errorf("empty assistant stream")
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "err", err)
		return acp.PromptResponse{}, err
	}
	stopReason := mapFinishReason(lastFinish)
	u := lastUsage
	if u == nil {
		u = &provider.Usage{
			Deltas:    deltas,
			ElapsedMs: ptrInt64(time.Since(streamStart).Milliseconds()),
		}
		if gotTTFT {
			u.TTFTMs = ptrInt64(ttftMs)
		}
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
	assistantMsg := runtime.Message{Role: "assistant", Content: contentText}
	if bound {
		if _, err := a.catalog.CommitTurn(ctx, sess.ThreadID, text, catalog.AssistantTurn{
			Content:      contentText,
			Model:        sess.Pin.CurrentModel,
			ProviderID:   sess.Pin.ProviderID,
			ProviderName: sess.Pin.ProviderName,
			StopReason:   string(stopReason),
			Parts:        turnParts(thought.String(), contentText, *u),
		}); err != nil {
			slog.Error("session/prompt failed", "session", sid, "err", err)
			return acp.PromptResponse{}, err
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

func turnParts(thought, message string, u provider.Usage) []catalog.MessagePart {
	parts := make([]catalog.MessagePart, 0, 3)
	if thought != "" {
		parts = append(parts, catalog.MessagePart{Type: "thought", Text: thought})
	}
	parts = append(parts, catalog.MessagePart{Type: "message", Text: message})
	deltas := u.Deltas
	parts = append(parts, catalog.MessagePart{
		Type:               "usage",
		PromptTokens:       u.PromptTokens,
		PredictedPerSecond: u.PredictedPerSecond,
		TTFTMs:             u.TTFTMs,
		Deltas:             &deltas,
	})
	return parts
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
