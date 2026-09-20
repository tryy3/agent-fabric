package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
	mounts, err := workspaceMounts(*opts.Docker, opts.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	identity := key
	if strings.TrimSpace(opts.Docker.Name) != "" {
		identity = opts.Docker.Name
	}
	containerID, err := manager.Acquire(ctx, key, container.ContainerSpec{
		Image:         image,
		Mounts:        mounts,
		WorkspaceRoot: opts.WorkspaceRoot,
		IdleTTL:       dockerIdleTTL(*opts.Docker),
		Name:          opts.Docker.Name,
	})
	if err != nil {
		return nil, err
	}

	executor := &containerExecutor{
		containerID:   containerID,
		bin:           bin,
		workspaceRoot: opts.WorkspaceRoot,
		runner:        runner,
		touch:         func() { manager.Touch(identity) },
	}
	return &environment{
		containerID: containerID,
		fs:          execfs.New(executor, opts.WorkspaceRoot),
		exec:        executor,
		close:       func() { manager.Done(identity) },
	}, nil
}

func scopeKey(scope sandboxcore.Scope) (string, error) {
	switch scope.Kind {
	case sandboxcore.ScopeShared:
		if scope.EnvironmentID != "" {
			return fmt.Sprintf("env:%s", scope.EnvironmentID), nil
		}
		return string(sandboxcore.ScopeShared), nil
	case sandboxcore.ScopeSession:
		if scope.SessionID == "" {
			return "", errors.New("docker session scope requires session ID")
		}
		return fmt.Sprintf("session:%s", scope.SessionID), nil
	case sandboxcore.ScopeProject:
		if scope.ProjectID == "" {
			return "", errors.New("docker project scope requires project ID")
		}
		return fmt.Sprintf("project:%s", scope.ProjectID), nil
	default:
		return "", fmt.Errorf("unsupported docker scope %q", scope.Kind)
	}
}

func workspaceMounts(
	opts sandboxcore.DockerOptions,
	workspaceRoot string,
) ([]sandboxcore.Mount, error) {
	overlayHasWorkspace := false
	for _, mount := range opts.Mounts {
		if isVolumeMount(mount) && mount.Target == workspaceRoot {
			overlayHasWorkspace = true
			break
		}
	}

	inject := ""
	replaceWorkspace := false
	if strings.TrimSpace(opts.WorkspaceVolume) != "" {
		inject = strings.TrimSpace(opts.WorkspaceVolume)
		replaceWorkspace = true
	} else if !overlayHasWorkspace {
		name, err := implicitWorkspaceVolumeName(opts)
		if err != nil {
			return nil, err
		}
		if name != "" {
			inject = name
			replaceWorkspace = true
		}
	}

	mounts := make([]sandboxcore.Mount, 0, len(opts.Mounts)+1)
	for _, mount := range opts.Mounts {
		if replaceWorkspace && mount.Target == workspaceRoot {
			continue
		}
		mounts = append(mounts, mount)
	}
	if inject != "" {
		mounts = append(mounts, sandboxcore.Mount{
			Source: inject,
			Target: workspaceRoot,
			Type:   sandboxcore.MountVolume,
		})
	}
	if !hasVolumeTarget(mounts, workspaceRoot) {
		return nil, fmt.Errorf("no enabled volume targets workspace root %q", workspaceRoot)
	}
	return mounts, nil
}

func workspaceVolumeName(opts sandboxcore.DockerOptions) (string, error) {
	if strings.TrimSpace(opts.WorkspaceVolume) != "" {
		return opts.WorkspaceVolume, nil
	}
	return implicitWorkspaceVolumeName(opts)
}

func implicitWorkspaceVolumeName(opts sandboxcore.DockerOptions) (string, error) {
	switch opts.Scope.Kind {
	case sandboxcore.ScopeProject:
		if opts.Scope.ProjectID == "" {
			return "", errors.New("docker project scope requires project ID")
		}
		return ProjectVolumeName(opts.Scope.ProjectID), nil
	case sandboxcore.ScopeShared:
		if opts.Scope.EnvironmentID == "" {
			return "", nil
		}
		return EnvironmentVolumeName(opts.Scope.EnvironmentID), nil
	default:
		return "", nil
	}
}

func isVolumeMount(mount sandboxcore.Mount) bool {
	return mount.Type == sandboxcore.MountVolume
}

func hasVolumeTarget(mounts []sandboxcore.Mount, target string) bool {
	for _, mount := range mounts {
		if isVolumeMount(mount) && mount.Target == target {
			return true
		}
	}
	return false
}

func dockerIdleTTL(opts sandboxcore.DockerOptions) time.Duration {
	if opts.IdleTTL != 0 {
		return opts.IdleTTL
	}
	switch opts.Scope.Kind {
	case sandboxcore.ScopeProject:
		return sandboxcore.DefaultProjectIdleTTL
	case sandboxcore.ScopeShared:
		if opts.Scope.EnvironmentID != "" {
			return sandboxcore.DefaultProjectIdleTTL
		}
	}
	return 0
}

func ProjectVolumeName(projectID string) string {
	return "agent-fabric.proj." + projectID
}

func EnvironmentVolumeName(environmentID string) string {
	return "agent-fabric.env." + environmentID
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
