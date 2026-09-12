package ws

import (
	"log/slog"
	"net/http"

	acp "github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/tryy3/agent-fabric/internal/agent"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func Handler(store *runtime.Store, catalogStore *catalog.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("acp websocket connecting", "remote", r.RemoteAddr)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("websocket upgrade", "remote", r.RemoteAddr, "err", err)
			return
		}

		bridge := NewBridge(conn)
		ag := agent.New(store, catalogStore)
		defer func() {
			ag.CloseConnectionSessions()
			slog.Info("acp websocket closed", "remote", r.RemoteAddr)
		}()
		asc := acp.NewAgentSideConnection(ag, bridge, bridge)
		ag.SetAgentConnection(asc)
		asc.SetLogger(slog.Default())

		<-asc.Done()
		_ = bridge.Close()
	})
}
