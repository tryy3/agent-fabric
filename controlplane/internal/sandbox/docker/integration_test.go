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
	"github.com/tryy3/agent-fabric/internal/sandbox/docker"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
)

func TestDockerFileToolsIntegration(t *testing.T) {
	skipWithoutRuntime(t)

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

func TestDockerProjectIsolation(t *testing.T) {
	skipWithoutRuntime(t)

	ctx := context.Background()
	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}

	projectA := "proj_itest_a"
	projectB := "proj_itest_b"
	envA, err := openProject(ctx, projectA)
	if err != nil {
		if isRuntimeInfrastructureError(err) {
			t.Skipf("docker/podman host unavailable: %v", err)
		}
		t.Fatal(err)
	}
	defer envA.Close(ctx)
	envB, err := openProject(ctx, projectB)
	if err != nil {
		t.Fatal(err)
	}
	defer envB.Close(ctx)
	t.Cleanup(func() {
		removeProjectVolume(t, projectA)
		removeProjectVolume(t, projectB)
	})

	if _, err := reg.Call(
		ctx,
		envA,
		"write_file",
		json.RawMessage(`{"path":"secret.txt","content":"from-a"}`),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Call(
		ctx,
		envB,
		"write_file",
		json.RawMessage(`{"path":"other.txt","content":"from-b"}`),
	); err != nil {
		t.Fatal(err)
	}

	out, err := reg.Call(ctx, envA, "read_file", json.RawMessage(`{"path":"secret.txt"}`))
	if err != nil || !strings.Contains(out, "from-a") {
		t.Fatalf("project A read own file: %s %v", out, err)
	}
	out, err = reg.Call(ctx, envB, "read_file", json.RawMessage(`{"path":"secret.txt"}`))
	if err == nil && strings.Contains(out, "from-a") {
		t.Fatalf("project B read project A's file: %s", out)
	}
	out, err = reg.Call(ctx, envA, "read_file", json.RawMessage(`{"path":"other.txt"}`))
	if err == nil && strings.Contains(out, "from-b") {
		t.Fatalf("project A read project B's file: %s", out)
	}
}

func openProject(ctx context.Context, projectID string) (sandbox.Environment, error) {
	return sandbox.Open(ctx, sandbox.OpenOptions{
		Kind:          "docker",
		WorkspaceRoot: "/workspace",
		Docker: &sandbox.DockerOptions{
			Scope: sandbox.Scope{
				Kind:      sandbox.ScopeProject,
				ProjectID: projectID,
			},
			Runtime: "auto",
			Image:   "alpine:3.20",
			IdleTTL: time.Minute,
		},
	})
}

func skipWithoutRuntime(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skip("no podman/docker")
		}
	}
}

func removeProjectVolume(t *testing.T, projectID string) {
	t.Helper()
	name := docker.ProjectVolumeName(projectID)
	bin := "docker"
	if _, err := exec.LookPath("podman"); err == nil {
		bin = "podman"
	} else if _, err := exec.LookPath("docker"); err != nil {
		return
	}
	_ = exec.Command(bin, "volume", "rm", "-f", name).Run()
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
