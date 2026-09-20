package docker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/container"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestEnvironmentExecRunsInsideContainerWorkspace(t *testing.T) {
	runner := &recordingExecRunner{
		result: CommandResult{
			ExitCode: 7,
			Stdout:   []byte("out"),
			Stderr:   []byte("err"),
		},
	}
	env := NewEnv("container-123", "/usr/bin/podman", "/workspace", runner)
	executor, ok := env.Exec()
	if !ok {
		t.Fatal("Exec() unavailable")
	}

	got, err := executor.Run(context.Background(), sandboxcore.ExecRequest{
		Cmd:   []string{"printf", "hello"},
		Stdin: []byte("input"),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{
		"exec", "-i", "-w", "/workspace", "container-123", "printf", "hello",
	}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
	}
	if string(runner.stdin) != "input" {
		t.Fatalf("stdin = %q", runner.stdin)
	}
	if !reflect.DeepEqual(got, sandboxcore.ExecResult{
		ExitCode: 7,
		Stdout:   []byte("out"),
		Stderr:   []byte("err"),
	}) {
		t.Fatalf("result = %#v", got)
	}
}

func TestScopeKey(t *testing.T) {
	tests := []struct {
		name    string
		scope   sandboxcore.Scope
		want    string
		wantErr string
	}{
		{
			name:  "shared without environment",
			scope: sandboxcore.Scope{Kind: sandboxcore.ScopeShared},
			want:  "shared",
		},
		{
			name: "shared with environment",
			scope: sandboxcore.Scope{
				Kind:          sandboxcore.ScopeShared,
				EnvironmentID: "env_tools",
			},
			want: "env:env_tools",
		},
		{
			name: "session",
			scope: sandboxcore.Scope{
				Kind:      sandboxcore.ScopeSession,
				SessionID: "sess-1",
			},
			want: "session:sess-1",
		},
		{
			name:    "session missing id",
			scope:   sandboxcore.Scope{Kind: sandboxcore.ScopeSession},
			wantErr: "session ID",
		},
		{
			name: "project",
			scope: sandboxcore.Scope{
				Kind:      sandboxcore.ScopeProject,
				ProjectID: "proj_abc",
			},
			want: "project:proj_abc",
		},
		{
			name:    "project missing id",
			scope:   sandboxcore.Scope{Kind: sandboxcore.ScopeProject},
			wantErr: "project ID",
		},
		{
			name:    "unsupported",
			scope:   sandboxcore.Scope{Kind: "process"},
			wantErr: "unsupported docker scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scopeKey(tt.scope)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("scopeKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenProjectScopeMountsNamedVolume(t *testing.T) {
	runner := &poolRunner{}
	manager := container.NewManager(runner, container.ManagerOptions{})
	env, err := openWithRunner(context.Background(), sandboxcore.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandboxcore.DockerOptions{
			Scope: sandboxcore.Scope{
				Kind:      sandboxcore.ScopeProject,
				ProjectID: "proj_abc",
			},
			Runtime: "docker",
			Image:   "alpine:3.20",
			Mounts: []sandboxcore.Mount{
				{Source: "./data", Target: "/workspace"},
				{Source: "/host/cache", Target: "/cache"},
			},
		},
	}, manager, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())

	if !runner.saw("agent-fabric.proj.proj_abc") {
		t.Fatal("expected named project volume")
	}
	if !runner.saw("type=volume,source=agent-fabric.proj.proj_abc,target=/workspace") {
		t.Fatal("expected volume mount at /workspace")
	}
	if runner.saw("type=bind,source=./data,target=/workspace") {
		t.Fatal("engine bind at /workspace should be replaced by the project volume")
	}
	if !runner.saw("type=bind,source=/host/cache,target=/cache") {
		t.Fatal("expected extra bind mounts to remain")
	}
}

func TestOpenPassesContainerName(t *testing.T) {
	runner := &poolRunner{}
	manager := container.NewManager(runner, container.ManagerOptions{})
	env, err := openWithRunner(context.Background(), sandboxcore.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandboxcore.DockerOptions{
			Scope: sandboxcore.Scope{
				Kind:      sandboxcore.ScopeProject,
				ProjectID: "proj_abc",
			},
			Runtime: "docker",
			Image:   "alpine:3.20",
			Name:    "agent-fabric-container-proj_abc",
		},
	}, manager, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	if !runner.saw("--name") || !runner.saw("agent-fabric-container-proj_abc") {
		t.Fatalf("missing --name in %#v", runner.commands)
	}
}

func TestWorkspaceVolumeName(t *testing.T) {
	got, err := workspaceVolumeName(sandboxcore.DockerOptions{
		Scope: sandboxcore.Scope{
			Kind:          sandboxcore.ScopeShared,
			EnvironmentID: "env_1",
		},
		WorkspaceVolume: "custom-vol",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "custom-vol" {
		t.Fatalf("explicit volume = %q", got)
	}

	got, err = workspaceVolumeName(sandboxcore.DockerOptions{
		Scope: sandboxcore.Scope{
			Kind:          sandboxcore.ScopeShared,
			EnvironmentID: "env_1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "agent-fabric.env.env_1" {
		t.Fatalf("env volume = %q", got)
	}
}

func TestEnvironmentProvidesExecBackedFS(t *testing.T) {
	runner := &recordingExecRunner{
		result: CommandResult{Stdout: []byte("contents")},
	}
	env := NewEnv("container-123", "/usr/bin/docker", "/workspace", runner)
	filesystem, ok := env.FS()
	if !ok {
		t.Fatal("FS() unavailable")
	}

	got, err := filesystem.ReadFile(context.Background(), "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "contents" {
		t.Fatalf("contents = %q", got)
	}
	wantPrefix := []string{
		"exec", "-w", "/workspace", "container-123",
	}
	if len(runner.args) < len(wantPrefix) ||
		!reflect.DeepEqual(runner.args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("args = %#v, want prefix %#v", runner.args, wantPrefix)
	}
}

type recordingExecRunner struct {
	result CommandResult
	name   string
	args   []string
	stdin  []byte
}

func (r *recordingExecRunner) Run(
	_ context.Context,
	name string,
	args []string,
	stdin []byte,
) (CommandResult, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	r.stdin = append([]byte(nil), stdin...)
	return r.result, nil
}
