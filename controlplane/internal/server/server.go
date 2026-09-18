package server

import (
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

func NewMux(
	store *runtime.Store,
	catalogStore *catalog.Store,
	sandboxOpts sandbox.OpenOptions,
) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, catalogStore, sandboxOpts))
	mux.Handle("/v1/", catalog.Handler(catalogStore))
	return withCORS(mux)
}

func New(
	addr string,
	store *runtime.Store,
	catalogStore *catalog.Store,
	sandboxOpts sandbox.OpenOptions,
) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: NewMux(store, catalogStore, sandboxOpts),
	}
}
