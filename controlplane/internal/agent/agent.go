package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type Agent struct {
	store    *runtime.Store
	streamer provider.ChatStreamer

	mu       sync.Mutex
	conn     *acp.AgentSideConnection
	sessions map[string]struct{}
	cancels  map[string]*context.CancelFunc
	closed   bool
}

func New(store *runtime.Store, streamer provider.ChatStreamer) *Agent {
	return &Agent{
		store:    store,
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
		return acp.NewSessionResponse{}, err
	}
	a.sessions[id] = struct{}{}
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
	if a.streamer == nil {
		return acp.PromptResponse{}, fmt.Errorf("streamer not configured")
	}

	text := provider.PromptText(params.Prompt)
	if err := a.store.Append(sid, runtime.Message{Role: "user", Content: text}); err != nil {
		return acp.PromptResponse{}, err
	}

	promptCtx, cancel := context.WithCancel(ctx)
	myCancel := &cancel
	a.mu.Lock()
	prev := a.cancels[sid]
	a.cancels[sid] = myCancel
	a.mu.Unlock()
	if prev != nil {
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
		return acp.PromptResponse{}, fmt.Errorf("session %s not found", sid)
	}

	var full strings.Builder
	err := a.streamer.StreamChat(promptCtx, msgs, func(delta string) error {
		full.WriteString(delta)
		return conn.SessionUpdate(promptCtx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update:    acp.UpdateAgentMessageText(delta),
		})
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	if full.Len() == 0 {
		return acp.PromptResponse{}, fmt.Errorf("empty assistant stream")
	}
	if err := a.store.Append(sid, runtime.Message{Role: "assistant", Content: full.String()}); err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *Agent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	sid := string(params.SessionId)
	a.mu.Lock()
	cf := a.cancels[sid]
	a.mu.Unlock()
	if cf != nil {
		(*cf)()
	}
	return nil
}

func (a *Agent) CloseSession(ctx context.Context, params acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	id := string(params.SessionId)
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
	invokeCancels(cfs)

	for _, id := range ids {
		a.store.Delete(id)
	}
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
