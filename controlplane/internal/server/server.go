package server

import (
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
	"github.com/tryy3/agent-fabric/internal/workspace"
)

func NewMux(
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine sandboxconfig.Engine,
) http.Handler {
	return NewMuxWithOpener(store, catalogStore, engine, workspace.NewCatalogOpener(catalogStore, engine))
}

func NewMuxWithOpener(
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine sandboxconfig.Engine,
	opener workspace.Opener,
) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, catalogStore, engine))
	workspace.Mount(mux, opener)
	mux.Handle("/v1/", catalog.Handler(catalogStore))
	return withCORS(mux)
}

func New(
	addr string,
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine sandboxconfig.Engine,
) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: NewMux(store, catalogStore, engine),
	}
}
