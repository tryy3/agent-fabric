package container

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestManager_ResolveBinary(t *testing.T) {
	tests := []struct {
		name    string
		paths   map[string]string
		binPath string
		runtime string
		want    string
		wantErr bool
	}{
		{
			name:    "explicit binary path",
			binPath: "/opt/bin/docker",
			runtime: "podman",
			want:    "/opt/bin/docker",
		},
		{
			name:    "named runtime",
			paths:   map[string]string{"docker": "/usr/bin/docker"},
			runtime: "docker",
			want:    "/usr/bin/docker",
		},
		{
			name: "auto prefers podman",
			paths: map[string]string{
				"podman": "/usr/bin/podman",
				"docker": "/usr/bin/docker",
			},
			runtime: "auto",
			want:    "/usr/bin/podman",
		},
		{
			name:    "auto falls back to docker",
			paths:   map[string]string{"docker": "/usr/bin/docker"},
			runtime: "",
			want:    "/usr/bin/docker",
		},
		{
			name:    "rejects unsupported runtime",
			runtime: "containerd",
			wantErr: true,
		},
		{
			name:    "auto reports missing runtimes",
			runtime: "auto",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.paths = tt.paths
			manager := NewManager(runner, ManagerOptions{})

			got, err := manager.ResolveBinary(tt.binPath, tt.runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveBinary() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveBinary() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveBinary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManager_AcquireReusesSameKey(t *testing.T) {
	runner := newFakeRunner()
	manager := NewManager(runner, ManagerOptions{IdleTTL: time.Minute})
	if _, err := manager.ResolveBinary("", "auto"); err != nil {
		t.Fatal(err)
	}
	spec := ContainerSpec{
		Image:         "alpine:3.20",
		WorkspaceRoot: "/workspace",
	}

	id1, err := manager.Acquire(context.Background(), "session:s1", spec)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := manager.Acquire(context.Background(), "session:s1", spec)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("same key IDs = %q and %q, want equal", id1, id2)
	}

	id3, err := manager.Acquire(context.Background(), "session:s2", spec)
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Fatalf("different key ID = %q, want different from %q", id3, id1)
	}
}

func TestManager_AcquireBuildsRunArguments(t *testing.T) {
	runner := newFakeRunner()
	manager := NewManager(runner, ManagerOptions{})
	if _, err := manager.ResolveBinary("", "auto"); err != nil {
		t.Fatal(err)
	}

	_, err := manager.Acquire(
		context.Background(),
		"session:s1",
		ContainerSpec{
			Image:         "alpine:3.20",
			WorkspaceRoot: "/workspace",
			Mounts: []sandboxcore.Mount{
				{Source: "/host/rw", Target: "/container/rw"},
				{
					Source:   "/host/ro",
					Target:   "/container/ro",
					ReadOnly: true,
				},
			},
			Labels: map[string]string{"team": "platform"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	run := runner.lastCommand("run")
	wantParts := []string{
		"run",
		"-d",
		"--workdir", "/workspace",
		"--label", "agent-fabric.sandbox.scope=session:s1",
		"--label", "team=platform",
		"--mount", "type=bind,source=/host/rw,target=/container/rw",
		"--mount", "type=bind,source=/host/ro,target=/container/ro,readonly",
		"alpine:3.20", "sleep", "infinity",
	}
	if got := strings.Join(run.args, " "); got != strings.Join(wantParts, " ") {
		t.Fatalf("run args = %q, want %q", got, strings.Join(wantParts, " "))
	}
}

func TestManager_AcquireCreatesNamedVolumeMount(t *testing.T) {
	runner := newFakeRunner()
	manager := NewManager(runner, ManagerOptions{})
	if _, err := manager.ResolveBinary("", "auto"); err != nil {
		t.Fatal(err)
	}

	_, err := manager.Acquire(
		context.Background(),
		"project:proj_abc",
		ContainerSpec{
			Image:         "alpine:3.20",
			WorkspaceRoot: "/workspace",
			IdleTTL:       sandboxcore.DefaultProjectIdleTTL,
			Mounts: []sandboxcore.Mount{
				{
					Source: "agent-fabric.proj.proj_abc",
					Target: "/workspace",
					Type:   sandboxcore.MountVolume,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	create := runner.lastCommand("volume")
	wantCreate := []string{"volume", "create", "agent-fabric.proj.proj_abc"}
	if got := strings.Join(create.args, " "); got != strings.Join(wantCreate, " ") {
		t.Fatalf("volume args = %q, want %q", got, strings.Join(wantCreate, " "))
	}

	run := runner.lastCommand("run")
	wantParts := []string{
		"run",
		"-d",
		"--workdir", "/workspace",
		"--label", "agent-fabric.sandbox.scope=project:proj_abc",
		"--mount", "type=volume,source=agent-fabric.proj.proj_abc,target=/workspace",
		"alpine:3.20", "sleep", "infinity",
	}
	if got := strings.Join(run.args, " "); got != strings.Join(wantParts, " ") {
		t.Fatalf("run args = %q, want %q", got, strings.Join(wantParts, " "))
	}
}

func TestManager_ReapUsesPerKeyIdleTTL(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	runner := newFakeRunner()
	manager := NewManager(runner, ManagerOptions{
		IdleTTL: time.Minute,
		Now:     func() time.Time { return now },
	})
	if _, err := manager.ResolveBinary("", "auto"); err != nil {
		t.Fatal(err)
	}

	projectID, err := manager.Acquire(
		context.Background(),
		"project:proj_a",
		ContainerSpec{
			Image:   "alpine:3.20",
			IdleTTL: sandboxcore.DefaultProjectIdleTTL,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := manager.Acquire(
		context.Background(),
		"session:s1",
		ContainerSpec{Image: "alpine:3.20", IdleTTL: time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	manager.Done("project:proj_a")
	manager.Done("session:s1")

	now = now.Add(2 * time.Minute)
	if err := manager.Reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.hasContainer(projectID) {
		t.Fatal("Reap() removed project container before 1h idle TTL")
	}
	if runner.hasContainer(sessionID) {
		t.Fatal("Reap() kept session container beyond 10m idle TTL")
	}
}

func TestManager_ReapWaitsForDoneAndRemovesIdleContainer(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	runner := newFakeRunner()
	manager := NewManager(runner, ManagerOptions{
		IdleTTL: time.Minute,
		Now:     func() time.Time { return now },
	})
	if _, err := manager.ResolveBinary("", "auto"); err != nil {
		t.Fatal(err)
	}

	id, err := manager.Acquire(
		context.Background(),
		"session:s1",
		ContainerSpec{Image: "alpine:3.20"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reusedID, acquireErr := manager.Acquire(
		context.Background(),
		"session:s1",
		ContainerSpec{Image: "alpine:3.20"},
	); acquireErr != nil {
		t.Fatal(acquireErr)
	} else if reusedID != id {
		t.Fatalf("second Acquire() ID = %q, want %q", reusedID, id)
	}
	now = now.Add(2 * time.Minute)
	if err := manager.Reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.hasContainer(id) {
		t.Fatal("Reap() removed busy container")
	}

	manager.Done("session:s1")
	if err := manager.Reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.hasContainer(id) {
		t.Fatal("Reap() removed container with one remaining reference")
	}

	manager.Done("session:s1")
	if err := manager.Reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.hasContainer(id) {
		t.Fatal("Reap() kept idle container after Done")
	}
}

func TestManager_TouchDefersReapingWithDefaultTTL(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	runner := newFakeRunner()
	manager := NewManager(runner, ManagerOptions{
		Now: func() time.Time { return now },
	})
	if _, err := manager.ResolveBinary("", "auto"); err != nil {
		t.Fatal(err)
	}

	id, err := manager.Acquire(
		context.Background(),
		"shared",
		ContainerSpec{Image: "alpine:3.20"},
	)
	if err != nil {
		t.Fatal(err)
	}
	manager.Done("shared")
	now = now.Add(9 * time.Minute)
	manager.Touch("shared")
	now = now.Add(2 * time.Minute)
	if err := manager.Reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.hasContainer(id) {
		t.Fatal("Reap() ignored Touch or default TTL")
	}

	now = now.Add(9 * time.Minute)
	if err := manager.Reap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.hasContainer(id) {
		t.Fatal("Reap() kept container beyond default TTL")
	}
}

type fakeRunner struct {
	mu         sync.Mutex
	paths      map[string]string
	containers map[string]fakeContainer
	volumes    map[string]struct{}
	commands   []fakeCommand
	nextID     int
}

type fakeContainer struct {
	id    string
	label string
}

type fakeCommand struct {
	name string
	args []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		paths:      map[string]string{"podman": "/usr/bin/podman"},
		containers: map[string]fakeContainer{},
		volumes:    map[string]struct{}{},
		commands:   []fakeCommand{},
	}
}

func (r *fakeRunner) LookPath(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if path := r.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (r *fakeRunner) CombinedOutput(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.commands = append(r.commands, fakeCommand{
		name: name,
		args: append([]string{}, args...),
	})
	switch args[0] {
	case "volume":
		if len(args) < 3 {
			return nil, fmt.Errorf("volume subcommand required")
		}
		name := args[len(args)-1]
		switch args[1] {
		case "inspect":
			if _, ok := r.volumes[name]; !ok {
				return nil, fmt.Errorf("no such volume %q", name)
			}
			return []byte(name + "\n"), nil
		case "create":
			r.volumes[name] = struct{}{}
			return []byte(name + "\n"), nil
		default:
			return nil, fmt.Errorf("unsupported volume command %q", args[1])
		}
	case "ps":
		label := strings.TrimPrefix(args[len(args)-1], "label=")
		for _, container := range r.containers {
			if container.label == label {
				return []byte(container.id + "\n"), nil
			}
		}
		return []byte{}, nil
	case "run":
		r.nextID++
		id := fmt.Sprintf("container-%d", r.nextID)
		label := argumentAfter(args, "--label")
		r.containers[id] = fakeContainer{id: id, label: label}
		return []byte(id + "\n"), nil
	case "rm":
		delete(r.containers, args[len(args)-1])
		return []byte{}, nil
	default:
		return nil, fmt.Errorf("unsupported command %q", args[0])
	}
}

func (r *fakeRunner) lastCommand(subcommand string) fakeCommand {
	r.mu.Lock()
	defer r.mu.Unlock()

	for index := len(r.commands) - 1; index >= 0; index-- {
		command := r.commands[index]
		if len(command.args) > 0 && command.args[0] == subcommand {
			return command
		}
	}
	return fakeCommand{}
}

func (r *fakeRunner) hasContainer(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.containers[id]
	return ok
}

func argumentAfter(args []string, name string) string {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}
