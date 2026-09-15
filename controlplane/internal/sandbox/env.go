package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/container"
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
)

func Open(ctx context.Context, opts OpenOptions) (Environment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch opts.Kind {
	case "local":
		return local.New(opts.WorkspaceRoot)
	case "docker":
		runner := docker.OSRunner{}
		var idleTTL time.Duration
		if opts.Docker != nil {
			idleTTL = opts.Docker.IdleTTL
		}
		manager := container.NewManager(runner, container.ManagerOptions{
			IdleTTL: idleTTL,
		})
		return docker.Open(ctx, opts, manager)
	default:
		return nil, fmt.Errorf("unsupported sandbox kind %q", opts.Kind)
	}
}
