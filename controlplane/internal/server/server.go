package server

import (
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
)

func NewMux(store *runtime.Store, catalogStore *catalog.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, catalogStore))
	mux.Handle("/v1/", catalog.Handler(catalogStore))
	return mux
}

func New(addr string, store *runtime.Store, catalogStore *catalog.Store) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: NewMux(store, catalogStore),
	}
}
