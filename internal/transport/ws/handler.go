package ws

import (
	"log/slog"
	"net/http"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/tryy3/agent-fabric/internal/agent"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func Handler(store *runtime.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("websocket upgrade", "err", err)
			return
		}

		bridge := NewBridge(conn)
		ag := agent.New(store)
		asc := acp.NewAgentSideConnection(ag, bridge, bridge)
		ag.SetAgentConnection(asc)
		asc.SetLogger(slog.Default())

		<-asc.Done()
		_ = bridge.Close()
	})
}
