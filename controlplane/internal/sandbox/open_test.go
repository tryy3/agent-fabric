package sandbox_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestOpenLocal(t *testing.T) {
	root := t.TempDir()
	env, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind: "local", WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	if !env.Caps().FS || !env.Caps().Exec {
		t.Fatalf("caps = %+v", env.Caps())
	}
}

func TestCapabilitiesSatisfies(t *testing.T) {
	caps := sandbox.Capabilities{FS: true}
	if !caps.Satisfies(sandbox.Capabilities{FS: true}) {
		t.Fatal("FS capability should satisfy FS requirement")
	}
	if caps.Satisfies(sandbox.Capabilities{Exec: true}) {
		t.Fatal("FS capability should not satisfy Exec requirement")
	}
}

func TestOpenDockerRequiresSessionID(t *testing.T) {
	_, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandbox.DockerOptions{
			Scope: sandbox.Scope{Kind: sandbox.ScopeSession},
			Image: "alpine:3.20",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "session ID") {
		t.Fatalf("err = %v", err)
	}
}
