package workspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func (o *CatalogOpener) openOptions(ctx context.Context, projectID string) (sandbox.OpenOptions, error) {
	if o == nil || o.Store == nil {
		return sandbox.OpenOptions{}, fmt.Errorf("catalog not configured")
	}
	project, err := o.Store.GetProject(ctx, projectID)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	return openProjectSandbox(ctx, o.Store, o.Engine, project)
}

func openProjectSandbox(
	ctx context.Context,
	store *catalog.Store,
	engine sandboxconfig.Engine,
	project catalog.Project,
) (sandbox.OpenOptions, error) {
	if strings.TrimSpace(project.ID) == "" {
		return sandbox.OpenOptions{}, fmt.Errorf("project %q has no resource", project.ID)
	}
	resolved, err := store.ResolveEnvironment(ctx, project.ID)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	if resolved.Resource == nil {
		if resolved.ResourceID != nil && strings.TrimSpace(*resolved.ResourceID) != "" {
			return sandbox.OpenOptions{}, fmt.Errorf("resource %q not found", strings.TrimSpace(*resolved.ResourceID))
		}
		return sandbox.OpenOptions{}, fmt.Errorf("project %q has no resource", project.ID)
	}
	return catalog.AttachSandboxOptions(resolved, project.ID, engine.Docker.Runtime, engine.Docker.BinPath)
}
