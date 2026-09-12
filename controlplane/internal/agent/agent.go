package agent

import (
	"context"
	"fmt"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type Agent struct {
	store *runtime.Store

	mu       sync.Mutex
	conn     *acp.AgentSideConnection
	sessions map[string]struct{}
	closed   bool
}

func New(store *runtime.Store) *Agent {
	return &Agent{
		store:    store,
		sessions: make(map[string]struct{}),
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
	text := provider.Echo(provider.PromptText(params.Prompt))
	if err := conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: params.SessionId,
		Update:    acp.UpdateAgentMessageText(text),
	}); err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *Agent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	return nil
}

func (a *Agent) CloseSession(ctx context.Context, params acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	id := string(params.SessionId)
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
	a.mu.Unlock()

	for _, id := range ids {
		a.store.Delete(id)
	}
}
