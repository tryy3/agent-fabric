package ws_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

func TestBridgeRoundTripLine(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		bridge := wstransport.NewBridge(c)
		// Agent-side: read one NDJSON line, write one back.
		line, err := bufio.NewReader(bridge).ReadString('\n')
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if _, err := io.WriteString(bridge, strings.TrimSpace(line)+"\n"); err != nil {
			t.Errorf("server write: %v", err)
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := wstransport.NewBridge(conn)
	if _, err := io.WriteString(client, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatalf("client write: %v", err)
	}
	got, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if strings.TrimSpace(got) != `{"jsonrpc":"2.0","id":1,"method":"ping"}` {
		t.Fatalf("got %q", got)
	}
}
