package docker

import (
	"context"
	"reflect"
	"testing"

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
