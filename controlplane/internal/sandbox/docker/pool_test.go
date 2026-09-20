package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestManagerPoolReapsClosedEnvironmentAfterIdleTTL(t *testing.T) {
	now := time.Date(2026, time.September, 16, 0, 0, 0, 0, time.UTC)
	runner := &poolRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool := newManagerPool(ctx, runner, time.Hour, func() time.Time {
		return now
	})
	opts := sandboxcore.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandboxcore.DockerOptions{
			Scope:   sandboxcore.Scope{Kind: sandboxcore.ScopeShared},
			IdleTTL: time.Minute,
			Runtime: "docker",
			Image:   "alpine:3.20",
			Mounts: []sandboxcore.Mount{{
				Source: "pool-workspace",
				Target: "/workspace",
				Type:   sandboxcore.MountVolume,
			}},
		},
	}

	env, err := pool.open(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := pool.reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.saw("rm") {
		t.Fatal("expected idle container removal")
	}
}

func TestManagerPoolSharesManagerAcrossIdleTTLs(t *testing.T) {
	now := time.Date(2026, time.September, 16, 0, 0, 0, 0, time.UTC)
	runner := &poolRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool := newManagerPool(ctx, runner, time.Hour, func() time.Time {
		return now
	})
	opts := sandboxcore.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandboxcore.DockerOptions{
			Scope:   sandboxcore.Scope{Kind: sandboxcore.ScopeShared},
			IdleTTL: time.Minute,
			Runtime: "docker",
			Image:   "alpine:3.20",
			Mounts: []sandboxcore.Mount{{
				Source: "pool-workspace",
				Target: "/workspace",
				Type:   sandboxcore.MountVolume,
			}},
		},
	}

	first, err := pool.open(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.Docker.IdleTTL = time.Hour
	second, err := pool.open(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := pool.reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.saw("rm") {
		t.Fatal("manager reaped a container still used by another environment")
	}

	if err := second.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pool.reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.saw("rm") {
		t.Fatal("expected the more aggressive idle TTL to reap the container")
	}
}

func TestManagerPoolOpenRejectsEmptyWorkspaceRoot(t *testing.T) {
	pool := newManagerPool(
		context.Background(),
		&poolRunner{},
		time.Hour,
		time.Now,
	)
	_, err := pool.open(context.Background(), sandboxcore.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: " \t\n",
		Docker: &sandboxcore.DockerOptions{
			Scope:   sandboxcore.Scope{Kind: sandboxcore.ScopeShared},
			Runtime: "docker",
			Image:   "alpine:3.20",
			Mounts: []sandboxcore.Mount{{
				Source: "pool-workspace",
				Target: "/workspace",
				Type:   sandboxcore.MountVolume,
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "workspace root") {
		t.Fatalf("err = %v", err)
	}
}

type poolRunner struct {
	mu       sync.Mutex
	commands []recordedCommand
}

func (r *poolRunner) LookPath(string) (string, error) {
	return "/usr/bin/docker", nil
}

func (r *poolRunner) CombinedOutput(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, recordedCommand{name: name, args: args})
	switch args[0] {
	case "run":
		return []byte("container-1\n"), nil
	case "rm":
		return nil, nil
	case "inspect":
		return nil, fmt.Errorf("no such container")
	default:
		return nil, nil
	}
}

func (r *poolRunner) Run(
	_ context.Context,
	_ string,
	_ []string,
	_ []byte,
) (CommandResult, error) {
	return CommandResult{}, nil
}

func (r *poolRunner) saw(arg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, command := range r.commands {
		for _, candidate := range command.args {
			if candidate == arg {
				return true
			}
		}
	}
	return false
}
