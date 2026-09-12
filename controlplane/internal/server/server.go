package server

import (
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

func NewMux(store *runtime.Store, catalogStore *catalog.Store, streamer provider.ChatStreamer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, catalogStore, streamer))
	mux.Handle("/v1/", catalog.Handler(catalogStore))
	return mux
}

func New(addr string, store *runtime.Store, catalogStore *catalog.Store, streamer provider.ChatStreamer) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: NewMux(store, catalogStore, streamer),
	}
}
