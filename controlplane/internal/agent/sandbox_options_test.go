package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func TestPromptSandboxOptionsUsesProjectScope(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker == nil {
		t.Fatal("docker options are nil")
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeProject || opts.Docker.Scope.ProjectID != project.ID {
		t.Fatalf("scope = %+v", opts.Docker.Scope)
	}
	if opts.Docker.Scope.SessionID != "" {
		t.Fatalf("SessionID overloaded: %+v", opts.Docker.Scope)
	}
	if opts.Docker.Image != catalog.DefaultSandboxImage {
		t.Fatalf("image = %q", opts.Docker.Image)
	}
	if opts.Docker.IdleTTL != sandbox.DefaultProjectIdleTTL {
		t.Fatalf("idle TTL = %v", opts.Docker.IdleTTL)
	}
}

func TestPromptSandboxOptionsUsesGlobalImagePatch(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPlaneSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PatchPlaneSettings(ctx, json.RawMessage(`{"image":"golang:1.23"}`)); err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker == nil || opts.Docker.Image != "golang:1.23" {
		t.Fatalf("image after global patch = %+v", opts.Docker)
	}
	if opts.Docker.IdleTTL != time.Hour {
		t.Fatalf("idle TTL should inherit: %v", opts.Docker.IdleTTL)
	}
}

func TestPromptSandboxOptionsKeepsSessionWithoutThread(t *testing.T) {
	ag := New(runtime.NewStore(), catalog.Open(dbtest.Open(t)), sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(context.Background(), runtime.Session{ID: "sess-9"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker == nil {
		t.Fatal("docker options are nil")
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeSession || opts.Docker.Scope.SessionID != "sess-9" {
		t.Fatalf("session scope = %+v", opts.Docker.Scope)
	}
}

func TestPromptSandboxOptionsLocalProjectWorkspace(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store := catalog.Open(dbtest.Open(t))
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{Kind: "local"}); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, "Notes", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{DataDir: dataDir})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := sandbox.ProjectWorkspaceRoot(dataDir, project.ID)
	if opts.WorkspaceRoot != want {
		t.Fatalf("workspace = %q, want %q", opts.WorkspaceRoot, want)
	}
	info, err := os.Stat(want)
	if err != nil || !info.IsDir() {
		t.Fatalf("workspace dir: %v", err)
	}
}

func TestPromptSandboxOptionsSharedUsesEnvironment(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	volume := "shared-tools-vol"
	environment, err := store.CreateEnvironment(ctx, "shared-tools", "docker", &volume)
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateSharedProject(ctx, "A", "", environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeShared || opts.Docker.Scope.EnvironmentID != environment.ID {
		t.Fatalf("shared scope = %+v", opts.Docker.Scope)
	}
	if opts.Docker.WorkspaceVolume != volume {
		t.Fatalf("volume = %q", opts.Docker.WorkspaceVolume)
	}
}

func TestPromptSandboxOptionsAgentOverlayWins(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "m1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	agentRow, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAgent(ctx, agentRow.ID, nil, nil, nil, nil, json.RawMessage(`{"sandbox":{"image":"busybox:1.36"}}`)); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
		Pin:      runtime.SessionPin{AgentID: agentRow.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker == nil || opts.Docker.Image != "busybox:1.36" {
		t.Fatalf("agent overlay image = %+v", opts.Docker)
	}
}

func TestProjectWorkspaceRootJoinsDataDir(t *testing.T) {
	got := sandbox.ProjectWorkspaceRoot(filepath.FromSlash("/data"), "proj_x")
	if got != filepath.Join("/data", "projects", "proj_x", "workspace") {
		t.Fatalf("got %q", got)
	}
}
