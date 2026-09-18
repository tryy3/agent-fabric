package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestResolveImageBuildsDockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(
		dockerfile,
		[]byte("FROM alpine:3.20\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}

	ref, err := ResolveImage(
		context.Background(),
		runner,
		"/usr/bin/podman",
		sandboxcore.DockerOptions{
			Dockerfile:   dockerfile,
			BuildContext: dir,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "agent-fabric-sandbox:") {
		t.Fatalf("ref = %q", ref)
	}
	if !runner.saw("build") {
		t.Fatal("expected build command")
	}
}

func TestResolveImageReusesExistingDockerfileImage(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM busybox\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{imageID: "sha256:existing\n"}

	ref, err := ResolveImage(
		context.Background(),
		runner,
		"/usr/bin/docker",
		sandboxcore.DockerOptions{Dockerfile: dockerfile},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "agent-fabric-sandbox:") {
		t.Fatalf("ref = %q", ref)
	}
	if runner.saw("build") {
		t.Fatal("did not expect build command")
	}
}

func TestResolveImageReturnsConfiguredImage(t *testing.T) {
	runner := &recordingRunner{}
	ref, err := ResolveImage(
		context.Background(),
		runner,
		"/usr/bin/docker",
		sandboxcore.DockerOptions{Image: "alpine:3.20"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if ref != "alpine:3.20" {
		t.Fatalf("ref = %q", ref)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	imageID  string
	commands []recordedCommand
}

func (r *recordingRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func (r *recordingRunner) CombinedOutput(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	r.commands = append(r.commands, recordedCommand{name: name, args: args})
	if len(args) > 0 && args[0] == "images" {
		return []byte(r.imageID), nil
	}
	return nil, nil
}

func (r *recordingRunner) Run(
	_ context.Context,
	_ string,
	_ []string,
	_ []byte,
) (CommandResult, error) {
	return CommandResult{}, nil
}

func (r *recordingRunner) saw(arg string) bool {
	for _, command := range r.commands {
		for _, candidate := range command.args {
			if candidate == arg {
				return true
			}
		}
	}
	return false
}
