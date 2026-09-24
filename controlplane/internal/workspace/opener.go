package workspace

import (
	"context"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

// Opener opens a project-scoped sandbox environment for catalog FS HTTP.
type Opener interface {
	Open(ctx context.Context, projectID string) (sandbox.Environment, error)
}

// CatalogOpener folds global + project sandbox overlay (no agent, no ACP session)
// and opens the environment the Files pane and preview should see.
type CatalogOpener struct {
	Store  *catalog.Store
	Engine sandboxconfig.Engine
}

func NewCatalogOpener(store *catalog.Store, engine sandboxconfig.Engine) *CatalogOpener {
	return &CatalogOpener{Store: store, Engine: engine}
}

func (o *CatalogOpener) Open(ctx context.Context, projectID string) (sandbox.Environment, error) {
	opts, err := o.openOptions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return sandbox.Open(ctx, opts)
}
