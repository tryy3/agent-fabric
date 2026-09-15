package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/tryy3/agent-fabric/internal/sandbox/container"
	"github.com/tryy3/agent-fabric/internal/sandbox/execfs"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type environment struct {
	containerID string
	fs          sandboxcore.FS
	exec        sandboxcore.Executor
	close       func()
	closeOnce   sync.Once
}

func NewEnv(
	containerID string,
	bin string,
	workspaceRoot string,
	runner CommandRunner,
) sandboxcore.Environment {
	executor := &containerExecutor{
		containerID:   containerID,
		bin:           bin,
		workspaceRoot: workspaceRoot,
		runner:        runner,
	}
	return &environment{
		containerID: containerID,
		fs:          execfs.New(executor, workspaceRoot),
		exec:        executor,
	}
}

func Open(
	ctx context.Context,
	opts sandboxcore.OpenOptions,
	manager *container.Manager,
) (sandboxcore.Environment, error) {
	return openWithRunner(ctx, opts, manager, OSRunner{})
}

func openWithRunner(
	ctx context.Context,
	opts sandboxcore.OpenOptions,
	manager *container.Manager,
	runner runtimeRunner,
) (sandboxcore.Environment, error) {
	if opts.Docker == nil {
		return nil, errors.New("docker options are required")
	}
	if manager == nil {
		return nil, errors.New("container manager is required")
	}
	key, err := scopeKey(opts.Docker.Scope)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.WorkspaceRoot) == "" {
		return nil, errors.New("docker workspace root is required")
	}

	bin, err := manager.ResolveBinary(
		opts.Docker.BinPath,
		opts.Docker.Runtime,
	)
	if err != nil {
		return nil, err
	}
	image, err := ResolveImage(ctx, runner, bin, *opts.Docker)
	if err != nil {
		return nil, err
	}
	containerID, err := manager.Acquire(ctx, key, container.ContainerSpec{
		Image:         image,
		Mounts:        opts.Docker.Mounts,
		WorkspaceRoot: opts.WorkspaceRoot,
	})
	if err != nil {
		return nil, err
	}

	executor := &containerExecutor{
		containerID:   containerID,
		bin:           bin,
		workspaceRoot: opts.WorkspaceRoot,
		runner:        runner,
		touch:         func() { manager.Touch(key) },
	}
	return &environment{
		containerID: containerID,
		fs:          execfs.New(executor, opts.WorkspaceRoot),
		exec:        executor,
		close:       func() { manager.Done(key) },
	}, nil
}

func scopeKey(scope sandboxcore.Scope) (string, error) {
	switch scope.Kind {
	case sandboxcore.ScopeShared:
		return string(sandboxcore.ScopeShared), nil
	case sandboxcore.ScopeSession:
		if scope.SessionID == "" {
			return "", errors.New("docker session scope requires session ID")
		}
		return fmt.Sprintf("session:%s", scope.SessionID), nil
	default:
		return "", fmt.Errorf("unsupported docker scope %q", scope.Kind)
	}
}

func (e *environment) ID() string {
	return e.containerID
}

func (e *environment) Caps() sandboxcore.Capabilities {
	return sandboxcore.Capabilities{FS: true, Exec: true}
}

func (e *environment) FS() (sandboxcore.FS, bool) {
	return e.fs, true
}

func (e *environment) Exec() (sandboxcore.Executor, bool) {
	return e.exec, true
}

func (e *environment) Close(context.Context) error {
	e.closeOnce.Do(func() {
		if e.close != nil {
			e.close()
		}
	})
	return nil
}
