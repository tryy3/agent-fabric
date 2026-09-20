package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/docker"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type Environment = sandboxcore.Environment
type ScopeKind = sandboxcore.ScopeKind
type Scope = sandboxcore.Scope
type Mount = sandboxcore.Mount
type DockerOptions = sandboxcore.DockerOptions
type OpenOptions = sandboxcore.OpenOptions

const (
	ScopeShared  = sandboxcore.ScopeShared
	ScopeSession = sandboxcore.ScopeSession
	ScopeProject = sandboxcore.ScopeProject

	DefaultSessionIdleTTL = sandboxcore.DefaultSessionIdleTTL
	DefaultProjectIdleTTL = sandboxcore.DefaultProjectIdleTTL

	MountBind   = sandboxcore.MountBind
	MountVolume = sandboxcore.MountVolume
)

var (
	ValidateContainerName = sandboxcore.ValidateContainerName
	ValidateVolumeName    = sandboxcore.ValidateVolumeName
)

// ProjectWorkspaceRoot is the local-kind jail for an isolated project.
func ProjectWorkspaceRoot(dataDir, projectID string) string {
	return filepath.Join(dataDir, "projects", projectID, "workspace")
}

func Open(ctx context.Context, opts OpenOptions) (Environment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.WorkspaceRoot) == "" {
		return nil, errors.New("sandbox workspace root is required")
	}
	switch opts.Kind {
	case "local":
		return local.New(opts.WorkspaceRoot)
	case "docker":
		return docker.OpenDefault(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported sandbox kind %q", opts.Kind)
	}
}
