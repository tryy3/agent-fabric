package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type Agent struct {
	store    *runtime.Store
	catalog  *catalog.Store
	streamer provider.ChatStreamer

	mu       sync.Mutex
	conn     *acp.AgentSideConnection
	sessions map[string]struct{}
	cancels  map[string]*context.CancelFunc
	closed   bool
}

func New(store *runtime.Store, catalogStore *catalog.Store, streamer provider.ChatStreamer) *Agent {
	return &Agent{
		store:    store,
		catalog:  catalogStore,
		streamer: streamer,
		sessions: make(map[string]struct{}),
		cancels:  make(map[string]*context.CancelFunc),
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
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
		},
	}, nil
}

func (a *Agent) NewSession(ctx context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return acp.NewSessionResponse{}, fmt.Errorf("connection closed")
	}
	id, err := a.store.Create(runtime.EchoDefinition())
	if err != nil {
		slog.Error("session/new failed", "err", err)
		return acp.NewSessionResponse{}, err
	}
	a.sessions[id] = struct{}{}
	slog.Info("session/new", "session", id)
	return acp.NewSessionResponse{SessionId: acp.SessionId(id)}, nil
}

func (a *Agent) Authenticate(ctx context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *Agent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	sid := string(params.SessionId)
	if _, ok := a.store.Get(sid); !ok {
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
	if a.streamer == nil {
		err := fmt.Errorf("streamer not configured")
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}

	text := provider.PromptText(params.Prompt)
	slog.Info("session/prompt start",
		"session", sid,
		"user_chars", len(text),
		"user_preview", preview(text, 80),
	)
	if err := a.store.Append(sid, runtime.Message{Role: "user", Content: text}); err != nil {
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
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

	msgs, ok := a.store.Messages(sid)
	if !ok {
		err := fmt.Errorf("session %s not found", sid)
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}

	var full strings.Builder
	var deltas int
	err := a.streamer.StreamChat(promptCtx, msgs, func(delta string) error {
		deltas++
		full.WriteString(delta)
		return conn.SessionUpdate(promptCtx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update:    acp.UpdateAgentMessageText(delta),
		})
	})
	if err != nil {
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "deltas", deltas, "err", err)
		return acp.PromptResponse{}, err
	}
	if full.Len() == 0 {
		err := fmt.Errorf("empty assistant stream")
		slog.Error("session/prompt failed", "session", sid, "history_msgs", len(msgs), "err", err)
		return acp.PromptResponse{}, err
	}
	if err := a.store.Append(sid, runtime.Message{Role: "assistant", Content: full.String()}); err != nil {
		slog.Error("session/prompt failed", "session", sid, "err", err)
		return acp.PromptResponse{}, err
	}
	slog.Info("session/prompt complete",
		"session", sid,
		"history_msgs", len(msgs)+1,
		"deltas", deltas,
		"assistant_chars", full.Len(),
		"assistant_preview", preview(full.String(), 80),
	)
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

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
