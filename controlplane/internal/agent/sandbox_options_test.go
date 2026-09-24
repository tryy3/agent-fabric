package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "agent-fabric-container-"+project.ID, "agent-fabric-vol-"+project.ID)

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
	wantName := "agent-fabric-container-" + project.ID
	if opts.Docker.Name != wantName {
		t.Fatalf("container name = %q, want %q", opts.Docker.Name, wantName)
	}
}

func TestPromptSandboxOptionsUsesGlobalImagePatch(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	resource := createContainerResource(t, store, "golang:1.23", "global-box", "global-disk", "/workspace")
	if _, err := store.GetPlaneSettings(ctx); err != nil {
		t.Fatal(err)
	}
	envPatch, err := json.Marshal(map[string]string{"resourceId": resource.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PatchPlaneSettings(ctx, nil, envPatch); err != nil {
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
	if opts.Kind != "" || opts.Docker != nil {
		t.Fatalf("unbound session attached a sandbox: %+v", opts)
	}
}

func TestPromptSandboxOptionsStaticNameSharedAcrossProjects(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	a, err := store.CreateProject(ctx, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject(ctx, "B", "")
	if err != nil {
		t.Fatal(err)
	}
	threadA, err := store.CreateThreadForProject(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	threadB, err := store.CreateThreadForProject(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	shared := createContainerResource(t, store, catalog.DefaultSandboxImage, "shared-build-box", "shared-disk", "/workspace")
	linkProjectResource(t, store, a.ID, shared.ID, "")
	linkProjectResource(t, store, b.ID, shared.ID, "")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	optsA, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-a", ThreadID: threadA.ID})
	if err != nil {
		t.Fatal(err)
	}
	optsB, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-b", ThreadID: threadB.ID})
	if err != nil {
		t.Fatal(err)
	}
	if optsA.Docker.Name != "shared-build-box" || optsB.Docker.Name != "shared-build-box" {
		t.Fatalf("names = %q %q", optsA.Docker.Name, optsB.Docker.Name)
	}
}

func TestPromptSandboxOptionsProjectTemplatesDoNotShare(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	a, err := store.CreateProject(ctx, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject(ctx, "B", "")
	if err != nil {
		t.Fatal(err)
	}
	threadA, err := store.CreateThreadForProject(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	threadB, err := store.CreateThreadForProject(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, a.ID, catalog.DefaultSandboxImage, "agent-fabric-container-"+a.ID, "agent-fabric-vol-"+a.ID)
	linkWorkspaceResource(t, store, b.ID, catalog.DefaultSandboxImage, "agent-fabric-container-"+b.ID, "agent-fabric-vol-"+b.ID)

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	optsA, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-a", ThreadID: threadA.ID})
	if err != nil {
		t.Fatal(err)
	}
	optsB, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-b", ThreadID: threadB.ID})
	if err != nil {
		t.Fatal(err)
	}
	if optsA.Docker.Name == optsB.Docker.Name {
		t.Fatalf("project templates collided: %q", optsA.Docker.Name)
	}
	if optsA.Docker.Name != "agent-fabric-container-"+a.ID || optsB.Docker.Name != "agent-fabric-container-"+b.ID {
		t.Fatalf("names = %q %q", optsA.Docker.Name, optsB.Docker.Name)
	}
}

func TestPromptSandboxOptionsAppliesIdentityPrefix(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	store.IdentityPrefix = "dev-"
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "agent-fabric-container-"+project.ID, "agent-fabric-vol-"+project.ID)
	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{
		Docker: sandboxconfig.DockerEngine{IdentityPrefix: "dev-"},
	})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := "dev-agent-fabric-container-" + project.ID
	if opts.Docker.Name != want {
		t.Fatalf("name = %q, want %q", opts.Docker.Name, want)
	}
}

func TestPromptSandboxOptionsRejectsUserIDTemplate(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	_, err := store.CreateResource(context.Background(), "Box", catalog.KindContainer, json.RawMessage(`{
		"image":"alpine:3.20",
		"containerName":"box-{userID}",
		"volumes":[{"id":"vol_0123456789abcdef","enabled":true,"name":"disk","target":"/workspace"}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "{userID}") {
		t.Fatalf("err = %v", err)
	}
}

func TestPromptSandboxOptionsLocalProjectWorkspace(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{Kind: "local"}); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, "Notes", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "notes-box", "notes-disk")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{DataDir: t.TempDir()})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Kind != "docker" || opts.WorkspaceRoot != "/workspace" || opts.Docker == nil || opts.Docker.Name != "notes-box" {
		t.Fatalf("options = %+v", opts)
	}
	grant := pathGrantAt(t, opts.PathPolicy, "/workspace")
	if !grant.Read || !grant.Write {
		t.Fatalf("workspace grant = %+v", grant)
	}
}

func TestPromptSandboxOptionsSharedUsesEnvironment(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "shared-box", "shared-tools-vol")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeProject || opts.Docker.Scope.ProjectID != project.ID {
		t.Fatalf("scope = %+v", opts.Docker.Scope)
	}
	got := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	if got.Source != "shared-tools-vol" {
		t.Fatalf("volume = %+v", got)
	}
}

func TestPromptSandboxOptionsDefaultWorkspaceVolume(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	seedFreshPlaneSettings(t, store)
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "agent-fabric-container-"+project.ID, "agent-fabric-vol-"+project.ID)

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	want := "agent-fabric-vol-" + project.ID
	if got.Source != want || got.Type != sandbox.MountVolume || got.ReadOnly {
		t.Fatalf("workspace mount = %+v, want source %q", got, want)
	}
}

func TestPromptSandboxOptionsPreservesPhase1VolumeOnUpgrade(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	projects, err := store.ListProjects(ctx)
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects: %v %+v", err, projects)
	}
	project := projects[0]
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	want := "agent-fabric-vol-" + project.ID
	if got.Source != want {
		t.Fatalf("backfilled volume = %q, want %q", got.Source, want)
	}
}

func TestPromptSandboxOptionsSharesStaticVolumeName(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	a, err := store.CreateProject(ctx, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject(ctx, "B", "")
	if err != nil {
		t.Fatal(err)
	}
	threadA, err := store.CreateThreadForProject(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	threadB, err := store.CreateThreadForProject(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	shared := createContainerResource(t, store, catalog.DefaultSandboxImage, "files-box", "shared-files", "/workspace")
	linkProjectResource(t, store, a.ID, shared.ID, "")
	linkProjectResource(t, store, b.ID, shared.ID, "")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	optsA, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-a", ThreadID: threadA.ID})
	if err != nil {
		t.Fatal(err)
	}
	optsB, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-b", ThreadID: threadB.ID})
	if err != nil {
		t.Fatal(err)
	}
	if volumeMountAt(t, optsA.Docker.Mounts, "/workspace").Source != "shared-files" {
		t.Fatalf("project A volume = %+v", optsA.Docker.Mounts)
	}
	if volumeMountAt(t, optsB.Docker.Mounts, "/workspace").Source != "shared-files" {
		t.Fatalf("project B volume = %+v", optsB.Docker.Mounts)
	}
}

func TestPromptSandboxOptionsMountsExtraTargetAndReadonly(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	seedFreshPlaneSettings(t, store)
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := json.Marshal(map[string]any{
		"image":         catalog.DefaultSandboxImage,
		"containerName": "landing-box",
		"volumes": []map[string]any{
			{"id": "vol_0123456789abcdef", "enabled": true, "name": "agent-fabric-vol-" + project.ID, "target": "/workspace", "whitelisted": true, "read": true, "write": true, "exec": true},
			{"id": "vol_0123456789abcd00", "enabled": true, "name": "cache-" + project.ID, "target": "/cache", "whitelisted": true, "read": true, "write": false, "exec": true},
			{"id": "vol_0123456789abcd01", "enabled": true, "name": "thread-" + thread.ID, "target": "/thread", "whitelisted": true, "read": true, "write": true, "exec": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := store.CreateResource(ctx, "Landing box", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	linkProjectResource(t, store, project.ID, resource.ID, "")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	if workspace.Source != "agent-fabric-vol-"+project.ID || workspace.ReadOnly {
		t.Fatalf("workspace = %+v", workspace)
	}
	cache := volumeMountAt(t, opts.Docker.Mounts, "/cache")
	if cache.Source != "cache-"+project.ID || !cache.ReadOnly || cache.Type != sandbox.MountVolume {
		t.Fatalf("cache = %+v", cache)
	}
	threadMount := volumeMountAt(t, opts.Docker.Mounts, "/thread")
	if threadMount.Source != "thread-"+thread.ID || threadMount.ReadOnly {
		t.Fatalf("thread volume = %+v", threadMount)
	}
}

func TestPromptSandboxOptionsFailsWithoutWorkspaceVolume(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	resource := createContainerResource(t, store, catalog.DefaultSandboxImage, "cache-box", "cache", "/cache")
	linkProjectResource(t, store, project.ID, resource.ID, "/workspace")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	_, err = ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err == nil || !strings.Contains(err.Error(), "workspace root") {
		t.Fatalf("err = %v", err)
	}
}

func TestPromptSandboxOptionsPrefixesVolumeNames(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	seedFreshPlaneSettings(t, store)
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	store.IdentityPrefix = "dev-"
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "agent-fabric-container-"+project.ID, "agent-fabric-vol-"+project.ID)
	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{
		Docker: sandboxconfig.DockerEngine{IdentityPrefix: "dev-"},
	})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	want := "dev-agent-fabric-vol-" + project.ID
	if got.Source != want {
		t.Fatalf("volume = %q, want %q", got.Source, want)
	}
}

func TestPromptSandboxOptionsSharedKeepsExtraVolume(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := json.Marshal(map[string]any{
		"image":         catalog.DefaultSandboxImage,
		"containerName": "shared-box",
		"volumes": []map[string]any{
			{"id": "vol_0123456789abcdef", "enabled": true, "name": "shared-tools-vol", "target": "/workspace", "whitelisted": true, "read": true, "write": true, "exec": true},
			{"id": "vol_0123456789abcd00", "enabled": true, "name": "cache", "target": "/cache", "whitelisted": true, "read": true, "write": true, "exec": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := store.CreateResource(ctx, "Shared", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	linkProjectResource(t, store, project.ID, resource.ID, "")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	if volumeMountAt(t, opts.Docker.Mounts, "/workspace").Source != "shared-tools-vol" {
		t.Fatalf("workspace mount = %+v", opts.Docker.Mounts)
	}
	if volumeMountAt(t, opts.Docker.Mounts, "/cache").Source != "cache" {
		t.Fatalf("extra mount missing: %+v", opts.Docker.Mounts)
	}
}

func TestPromptSandboxOptionsLocalIgnoresVolumes(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{Kind: "local"}); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, "Notes", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "notes-box", "notes-disk")
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, json.RawMessage(`{
		"sandbox":{"volumes":[{"id":"vol_cache","name":"cache","target":"/cache","enabled":true}]}
	}`)); err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{DataDir: t.TempDir()})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker == nil || opts.Kind != "docker" {
		t.Fatalf("options = %+v", opts)
	}
	for _, mount := range opts.Docker.Mounts {
		if mount.Target == "/cache" {
			t.Fatalf("sandbox volume leaked into attach: %+v", opts.Docker.Mounts)
		}
	}
}

func volumeMountAt(t *testing.T, mounts []sandbox.Mount, target string) sandbox.Mount {
	t.Helper()
	for _, mount := range mounts {
		if mount.Type == sandbox.MountVolume && mount.Target == target {
			return mount
		}
	}
	t.Fatalf("no volume mount at %s in %+v", target, mounts)
	return sandbox.Mount{}
}

func pathGrantAt(t *testing.T, policy *sandbox.PathPolicy, path string) sandbox.PathGrant {
	t.Helper()
	if policy == nil {
		t.Fatal("path policy is nil")
	}
	for _, grant := range policy.Grants {
		if grant.Path == path {
			return grant
		}
	}
	t.Fatalf("no path grant at %s in %+v", path, policy.Grants)
	return sandbox.PathGrant{}
}

func seedFreshPlaneSettings(t *testing.T, store *catalog.Store) {
	t.Helper()
	if _, err := store.GetPlaneSettings(context.Background()); err != nil {
		t.Fatal(err)
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
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	linkWorkspaceResource(t, store, project.ID, catalog.DefaultSandboxImage, "coder-box", "coder-disk")

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{
		ID:       "sess-1",
		ThreadID: thread.ID,
		Pin:      runtime.SessionPin{AgentID: agentRow.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker == nil || opts.Docker.Image != catalog.DefaultSandboxImage {
		t.Fatalf("agent overlay image = %+v", opts.Docker)
	}
}

func TestPromptSandboxOptionsPathPolicyWhitelist(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	seedFreshPlaneSettings(t, store)
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := json.Marshal(map[string]any{
		"image":         catalog.DefaultSandboxImage,
		"containerName": "policy-box",
		"volumes": []map[string]any{
			{"id": "vol_0123456789abcdef", "enabled": true, "name": "ws", "target": "/workspace", "whitelisted": true, "read": true, "write": false, "exec": true},
			{"id": "vol_0123456789abcd00", "enabled": true, "name": "cache", "target": "/cache", "whitelisted": true, "read": true, "write": true, "exec": true},
			{"id": "vol_0123456789abcd01", "enabled": true, "name": "hidden", "target": "/secret", "whitelisted": false, "read": true, "write": true, "exec": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := store.CreateResource(ctx, "Policy", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := json.Marshal(map[string]any{
		"environment": map[string]any{
			"resourceId": resource.ID,
			"extraPaths": []map[string]any{
				{"id": "path_tmp", "path": "/tmp", "enabled": true, "whitelisted": true, "read": true, "write": true, "exec": false},
				{"id": "path_opt", "path": "/opt", "enabled": true},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, settings); err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	if opts.PathPolicy == nil {
		t.Fatal("path policy is nil")
	}
	workspace := pathGrantAt(t, opts.PathPolicy, "/workspace")
	if !workspace.Read || workspace.Write {
		t.Fatalf("workspace grant = %+v", workspace)
	}
	cache := pathGrantAt(t, opts.PathPolicy, "/cache")
	if !cache.Read || !cache.Write {
		t.Fatalf("cache grant = %+v", cache)
	}
	tmp := pathGrantAt(t, opts.PathPolicy, "/tmp")
	if !tmp.Write || tmp.Exec {
		t.Fatalf("extra grant = %+v", tmp)
	}
	for _, grant := range opts.PathPolicy.Grants {
		if grant.Path == "/secret" {
			t.Fatalf("non-whitelisted volume leaked into policy: %+v", grant)
		}
		if grant.Path == "/opt" {
			t.Fatalf("omitted extra-path flags became a grant: %+v", grant)
		}
	}
}

func TestProjectWorkspaceRootJoinsDataDir(t *testing.T) {
	got := sandbox.ProjectWorkspaceRoot(filepath.FromSlash("/data"), "proj_x")
	if got != filepath.Join("/data", "projects", "proj_x", "workspace") {
		t.Fatalf("got %q", got)
	}
}

func TestPromptSandboxUsesLinkedResource(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	resource := createContainerResource(t, store, "alpine:3.20", "box", "disk", "/workspace")
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	linkProjectResource(t, store, project.ID, resource.ID, "")
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	provider, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, provider.ID, []catalog.ModelInfo{{ID: "m1", Name: "m1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	agentRow, err := store.CreateAgent(ctx, "Coder", "", provider.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAgent(ctx, agentRow.ID, nil, nil, nil, nil, json.RawMessage(`{"sandbox":{"image":"debian:12"}}`)); err != nil {
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
	if opts.Docker == nil || opts.Docker.Image != "alpine:3.20" {
		t.Fatalf("image = %+v", opts.Docker)
	}
	if opts.Docker.Name != "box" {
		t.Fatalf("container name = %q", opts.Docker.Name)
	}
	got := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	if got.Source != "disk" || got.Type != sandbox.MountVolume {
		t.Fatalf("mount = %+v", got)
	}
	if opts.Docker.Image == "debian:12" {
		t.Fatal("agent image was used")
	}

	bare, err := store.CreateProject(ctx, "Bare", "")
	if err != nil {
		t.Fatal(err)
	}
	bareThread, err := store.CreateThreadForProject(ctx, bare.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-bare", ThreadID: bareThread.ID})
	if err == nil || !strings.Contains(err.Error(), "has no resource") {
		t.Fatalf("no resource err = %v", err)
	}

	mismatch := createContainerResource(t, store, "alpine:3.20", "data-box", "disk-data", "/data")
	mismatchProject, err := store.CreateProject(ctx, "Mismatch", "")
	if err != nil {
		t.Fatal(err)
	}
	linkProjectResource(t, store, mismatchProject.ID, mismatch.ID, "/workspace")
	mismatchThread, err := store.CreateThreadForProject(ctx, mismatchProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-mismatch", ThreadID: mismatchThread.ID})
	if err == nil || !strings.Contains(err.Error(), `no enabled volume targets workspace root "/workspace"`) {
		t.Fatalf("workspace root err = %v", err)
	}

	if err := store.DeleteProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	resources, err := store.ListResources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range resources {
		if row.ID == resource.ID {
			return
		}
	}
	t.Fatalf("resource %q missing after project delete", resource.ID)
}

func linkWorkspaceResource(t *testing.T, store *catalog.Store, projectID, image, containerName, volumeName string) catalog.Resource {
	t.Helper()
	resource := createContainerResource(t, store, image, containerName, volumeName, "/workspace")
	linkProjectResource(t, store, projectID, resource.ID, "")
	return resource
}

func createContainerResource(t *testing.T, store *catalog.Store, image, containerName, volumeName, target string) catalog.Resource {
	t.Helper()
	spec, err := json.Marshal(map[string]any{
		"image":         image,
		"containerName": containerName,
		"volumes": []map[string]any{{
			"id":          "vol_0123456789abcdef",
			"enabled":     true,
			"name":        volumeName,
			"target":      target,
			"whitelisted": true,
			"read":        true,
			"write":       true,
			"exec":        true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := store.CreateResource(context.Background(), containerName, catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	return resource
}

func linkProjectResource(t *testing.T, store *catalog.Store, projectID, resourceID, workspaceRoot string) {
	t.Helper()
	env := map[string]any{"resourceId": resourceID}
	if workspaceRoot != "" {
		env["workspaceRoot"] = workspaceRoot
	}
	raw, err := json.Marshal(map[string]any{"environment": env})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(context.Background(), projectID, nil, nil, raw); err != nil {
		t.Fatal(err)
	}
}
