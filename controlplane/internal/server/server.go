package server

import (
	"context"
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/integration"
	"github.com/tryy3/agent-fabric/internal/runtime"
	wstransport "github.com/tryy3/agent-fabric/internal/transport/ws"
	"github.com/tryy3/agent-fabric/internal/workspace"
)

func NewMux(
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine engineconfig.Engine,
) http.Handler {
	return NewMuxWithOpener(store, catalogStore, engine, workspace.NewCatalogOpener(catalogStore, engine))
}

func NewMuxWithOpener(
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine engineconfig.Engine,
	opener workspace.Opener,
) http.Handler {
	catalogStore.IdentityPrefix = engine.Docker.IdentityPrefix
	catalog.TestToolIntegrationFunc = func(ti catalog.ToolIntegration, secrets catalog.ToolIntegrationSecrets) error {
		return integration.TestConnection(context.Background(), ti, secrets)
	}
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, catalogStore, engine))
	workspace.MountWithStore(mux, opener, catalogStore)
	mux.Handle("/v1/", catalog.HandlerWithHooks(catalogStore, catalog.Hooks{
		AfterCreateProject: func(ctx context.Context, project catalog.Project) error {
			return workspace.InitRepo(ctx, opener, project.ID)
		},
	}))
	return withCORS(mux)
}

func New(
	addr string,
	store *runtime.Store,
	catalogStore *catalog.Store,
	engine engineconfig.Engine,
) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: NewMux(store, catalogStore, engine),
	}
}
