package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

type printClient struct{}

func (printClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	u := params.Update
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		fmt.Print(u.AgentMessageChunk.Content.Text.Text)
	}
	return nil
}

func (printClient) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, nil
}
func (printClient) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, nil
}
func (printClient) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, nil
}
func (printClient) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, nil
}
func (printClient) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, nil
}
func (printClient) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}
func (printClient) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}
func (printClient) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, nil
}

var _ acp.Client = printClient{}

func main() {
	addr := flag.String("addr", "localhost:8080", "control plane host:port")
	agentID := flag.String("agent-id", "", "catalog agent id (_meta.agentId)")
	prompt := flag.String("prompt", "hello", "user prompt text")
	flag.Parse()

	wsURL := "ws://" + *addr + "/acp"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	bridge := wstransport.NewBridge(conn)
	csc := acp.NewClientSideConnection(printClient{}, bridge, bridge)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		log.Fatalf("initialize: %v", err)
	}
	req := acp.NewSessionRequest{Cwd: mustCwd(), McpServers: []acp.McpServer{}}
	if *agentID != "" {
		req.Meta = map[string]any{"agentId": *agentID}
	}
	sess, err := csc.NewSession(ctx, req)
	if err != nil {
		log.Fatalf("session/new: %v", err)
	}
	if _, err := csc.Prompt(ctx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(*prompt)},
	}); err != nil {
		log.Fatalf("prompt: %v", err)
	}
	fmt.Println()
}

func mustCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "/"
	}
	return wd
}
