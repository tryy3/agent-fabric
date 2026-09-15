//go:build integration

package docker_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
)

func TestDockerFileToolsIntegration(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skip("no podman/docker")
		}
	}

	ctx := context.Background()
	env, err := sandbox.Open(ctx, sandbox.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandbox.DockerOptions{
			Scope: sandbox.Scope{
				Kind:      sandbox.ScopeSession,
				SessionID: "itest-1",
			},
			Runtime: "auto",
			Image:   "alpine:3.20",
			IdleTTL: time.Minute,
		},
	})
	if err != nil {
		if isRuntimeInfrastructureError(err) {
			t.Skipf("docker/podman host unavailable: %v", err)
		}
		t.Fatal(err)
	}
	defer env.Close(ctx)

	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}

	if _, err := reg.Call(
		ctx,
		env,
		"write_file",
		json.RawMessage(`{"path":"x.txt","content":"pod"}`),
	); err != nil {
		t.Fatal(err)
	}
	out, err := reg.Call(
		ctx,
		env,
		"read_file",
		json.RawMessage(`{"path":"x.txt"}`),
	)
	if err != nil || !strings.Contains(out, "pod") {
		t.Fatalf("%s %v", out, err)
	}
}

func isRuntimeInfrastructureError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"user namespace",
		"cannot re-exec",
		"permission denied",
		"operation not permitted",
		"cannot connect to the docker daemon",
		"is the docker daemon running",
		"podman machine",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
