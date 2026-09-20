package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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
	globalSettings, err := o.Store.GetPlaneSettings(ctx)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	global, err := catalog.DecodeOverlay(globalSettings.Sandbox)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	projectOverlay, err := overlayFromSettings(project.Settings)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	effective := catalog.ResolveOverlay(catalog.DefaultOverlay(false), global, projectOverlay)
	return openProjectSandbox(ctx, o.Store, o.Engine, effective, project)
}

func overlayFromSettings(raw json.RawMessage) (catalog.Overlay, error) {
	sandboxJSON, err := catalog.SandboxFromSettings(raw)
	if err != nil {
		return catalog.Overlay{}, err
	}
	return catalog.DecodeOverlay(sandboxJSON)
}

func openProjectSandbox(
	ctx context.Context,
	store *catalog.Store,
	engine sandboxconfig.Engine,
	effective catalog.Overlay,
	project catalog.Project,
) (sandbox.OpenOptions, error) {
	kind := catalog.DefaultSandboxKind
	if effective.Kind != nil && *effective.Kind != "" {
		kind = *effective.Kind
	}
	workspaceRoot := catalog.DefaultWorkspaceRoot
	if effective.WorkspaceRoot != nil && *effective.WorkspaceRoot != "" {
		workspaceRoot = *effective.WorkspaceRoot
	}
	opts := sandbox.OpenOptions{Kind: kind, WorkspaceRoot: workspaceRoot}
	if kind == "local" {
		dataDir := engine.DataDir
		if dataDir == "" {
			dataDir = "./data"
		}
		root := sandbox.ProjectWorkspaceRoot(dataDir, project.ID)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return sandbox.OpenOptions{}, fmt.Errorf("create project workspace: %w", err)
		}
		opts.WorkspaceRoot = root
		opts.PathPolicy = overlayPathPolicy(effective, kind, workspaceRoot, root)
		return opts, nil
	}
	if kind != "docker" {
		return opts, nil
	}

	image := catalog.DefaultSandboxImage
	if effective.Image != nil && *effective.Image != "" {
		image = *effective.Image
	}
	ttl := time.Duration(catalog.DefaultIdleTTLSeconds) * time.Second
	if effective.IdleTTLSeconds != nil {
		ttl = time.Duration(*effective.IdleTTLSeconds) * time.Second
	}
	opts.Docker = &sandbox.DockerOptions{
		IdleTTL:      ttl,
		Runtime:      engine.Docker.Runtime,
		BinPath:      engine.Docker.BinPath,
		Image:        image,
		Dockerfile:   derefString(effective.Dockerfile),
		BuildContext: derefString(effective.BuildContext),
	}
	template := catalog.DefaultContainerNameTemplate
	if effective.ContainerName != nil && strings.TrimSpace(*effective.ContainerName) != "" {
		template = strings.TrimSpace(*effective.ContainerName)
	}
	name, err := catalog.ExpandName(template, catalog.NameVars{ProjectID: project.ID})
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	name = catalog.ApplyIdentityPrefix(name, engine.Docker.IdentityPrefix)
	if err := sandbox.ValidateContainerName(name); err != nil {
		return sandbox.OpenOptions{}, err
	}
	opts.Docker.Name = name
	mounts, err := overlayVolumeMounts(effective, catalog.NameVars{ProjectID: project.ID}, engine.Docker.IdentityPrefix)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	opts.Docker.Mounts = mounts
	opts, err = applyProjectSandbox(ctx, store, opts, project)
	if err != nil {
		return sandbox.OpenOptions{}, err
	}
	if missingWorkspaceVolume(opts) {
		return sandbox.OpenOptions{}, fmt.Errorf("no enabled volume targets workspace root %q", opts.WorkspaceRoot)
	}
	opts.PathPolicy = overlayPathPolicy(effective, kind, workspaceRoot, opts.WorkspaceRoot)
	return opts, nil
}

func overlayPathPolicy(effective catalog.Overlay, kind, overlayRoot, hostRoot string) *sandbox.PathPolicy {
	grants := catalog.OverlayPathPolicy(effective, kind, overlayRoot, hostRoot)
	out := make([]sandbox.PathGrant, 0, len(grants))
	for _, grant := range grants {
		out = append(out, sandbox.PathGrant{
			Path:  grant.Path,
			Read:  grant.Read,
			Write: grant.Write,
			Exec:  grant.Exec,
		})
	}
	return &sandbox.PathPolicy{Grants: out}
}

func overlayVolumeMounts(effective catalog.Overlay, vars catalog.NameVars, prefix string) ([]sandbox.Mount, error) {
	resolved, err := catalog.ExpandVolumes(effective.Volumes, vars, prefix)
	if err != nil {
		return nil, err
	}
	mounts := make([]sandbox.Mount, 0, len(resolved))
	for _, volume := range resolved {
		if err := sandbox.ValidateVolumeName(volume.Name); err != nil {
			return nil, err
		}
		mounts = append(mounts, sandbox.Mount{
			Source:   volume.Name,
			Target:   volume.Target,
			Type:     sandbox.MountVolume,
			ReadOnly: volume.ReadOnly,
		})
	}
	return mounts, nil
}

func missingWorkspaceVolume(opts sandbox.OpenOptions) bool {
	if opts.Kind != "docker" || opts.Docker == nil {
		return false
	}
	if strings.TrimSpace(opts.Docker.WorkspaceVolume) != "" {
		return false
	}
	for _, mount := range opts.Docker.Mounts {
		if mount.Type == sandbox.MountVolume && mount.Target == opts.WorkspaceRoot {
			return false
		}
	}
	return true
}

func applyProjectSandbox(
	ctx context.Context,
	store *catalog.Store,
	opts sandbox.OpenOptions,
	project catalog.Project,
) (sandbox.OpenOptions, error) {
	if opts.Kind != "docker" || opts.Docker == nil {
		return opts, nil
	}
	if project.Isolation == catalog.IsolationShared {
		if project.EnvironmentID == nil || strings.TrimSpace(*project.EnvironmentID) == "" {
			return sandbox.OpenOptions{}, fmt.Errorf("shared isolation requires environmentId")
		}
		environment, err := store.GetEnvironment(ctx, *project.EnvironmentID)
		if err != nil {
			return sandbox.OpenOptions{}, err
		}
		opts.Docker.Scope = sandbox.Scope{
			Kind:          sandbox.ScopeShared,
			EnvironmentID: environment.ID,
		}
		if environment.VolumeName != nil && strings.TrimSpace(*environment.VolumeName) != "" {
			opts.Docker.WorkspaceVolume = *environment.VolumeName
		}
		return opts, nil
	}
	opts.Docker.Scope = sandbox.Scope{
		Kind:      sandbox.ScopeProject,
		ProjectID: project.ID,
	}
	return opts, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
