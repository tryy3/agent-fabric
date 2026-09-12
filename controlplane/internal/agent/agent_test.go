package agent_test

import (
	"context"
	"io"
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

func TestEchoTurnOverPipes(t *testing.T) {
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	store := runtime.NewStore()
	ag := agent.New(store)
	asc := acp.NewAgentSideConnection(ag, agentToClientW, clientToAgentR)
	ag.SetAgentConnection(asc)

	client := &captureClient{updates: make(chan struct{}, 8)}
	csc := acp.NewClientSideConnection(client, clientToAgentW, agentToClientR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		if joined == "hello" {
			return
		}
		select {
		case <-client.updates:
		case <-deadline:
			t.Fatalf("echo chunks = %q, want hello", joined)
		}
	}
}
