package ws_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/tryy3/agent-fabric/internal/runtime"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

type lifecycleClient struct {
	acp.Client
}

func TestHandlerDeletesConnectionSessionsOnDisconnect(t *testing.T) {
	store := runtime.NewStore()
	srv := httptest.NewServer(wstransport.Handler(store))
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(srv.URL, "http"),
		nil,
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	bridge := wstransport.NewBridge(conn)
	csc := acp.NewClientSideConnection(&lifecycleClient{}, bridge, bridge)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := csc.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sess, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sessionID := string(sess.SessionId)
	if _, ok := store.Get(sessionID); !ok {
		t.Fatal("session was not stored")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("close WebSocket: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := store.Get(sessionID); !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("session remains after WebSocket disconnect")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
