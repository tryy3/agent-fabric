package agent

import (
	"context"
	"encoding/json"
	"os"
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
	wantName := "agent-fabric-container-" + project.ID
	if opts.Docker.Name != wantName {
		t.Fatalf("container name = %q, want %q", opts.Docker.Name, wantName)
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
	if _, err := store.PatchPlaneSettings(ctx, json.RawMessage(`{"image":"golang:1.23"}`), nil); err != nil {
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
	_, err := ag.promptSandboxOptions(context.Background(), runtime.Session{ID: "sess-9"})
	if err == nil || !strings.Contains(err.Error(), "projectID") {
		t.Fatalf("unbound default template err = %v", err)
	}
}

func TestPromptSandboxOptionsStaticNameSharedAcrossProjects(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	a, err := store.CreateProject(ctx, "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject(ctx, "B", "", "")
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
	if _, err := store.UpdateProject(ctx, a.ID, nil, nil, nil, json.RawMessage(`{"sandbox":{"containerName":"shared-build-box"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, b.ID, nil, nil, nil, json.RawMessage(`{"sandbox":{"containerName":"shared-build-box"}}`)); err != nil {
		t.Fatal(err)
	}

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
	a, err := store.CreateProject(ctx, "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject(ctx, "B", "", "")
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
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
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
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{"sandbox":{"containerName":"box-{userID}"}}`)); err != nil {
		t.Fatal(err)
	}
	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	_, err = ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err == nil || !strings.Contains(err.Error(), "{userID}") {
		t.Fatalf("err = %v", err)
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
	grant := pathGrantAt(t, opts.PathPolicy, want)
	if !grant.Read || !grant.Write {
		t.Fatalf("local workspace grant = %+v", grant)
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

func TestPromptSandboxOptionsDefaultWorkspaceVolume(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	seedFreshPlaneSettings(t, store)
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
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
	if got.Source != want || got.Type != sandbox.MountVolume || got.ReadOnly {
		t.Fatalf("workspace mount = %+v, want source %q", got, want)
	}
}

func TestPromptSandboxOptionsPreservesPhase1VolumeOnUpgrade(t *testing.T) {
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
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := volumeMountAt(t, opts.Docker.Mounts, "/workspace")
	want := "agent-fabric.proj." + project.ID
	if got.Source != want {
		t.Fatalf("upgrade volume = %q, want %q", got.Source, want)
	}
}

func TestPromptSandboxOptionsSharesStaticVolumeName(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	a, err := store.CreateProject(ctx, "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateProject(ctx, "B", "", "")
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
	patch := json.RawMessage(`{"sandbox":{"volumes":[{"id":"vol_workspace","name":"shared-files"}]}}`)
	if _, err := store.UpdateProject(ctx, a.ID, nil, nil, nil, patch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, b.ID, nil, nil, nil, patch); err != nil {
		t.Fatal(err)
	}

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
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{
		"sandbox":{"volumes":[
			{"id":"vol_cache","name":"cache-{projectID}","target":"/cache","enabled":true,"write":false},
			{"id":"vol_thread","name":"thread-{threadID}","target":"/thread","enabled":true}
		]}
	}`)); err != nil {
		t.Fatal(err)
	}

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
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{
		"sandbox":{"volumes":[{"id":"vol_workspace","enabled":false},{"id":"vol_cache","name":"cache","target":"/cache","enabled":true}]}
	}`)); err != nil {
		t.Fatal(err)
	}

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
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
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
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{
		"sandbox":{"volumes":[{"id":"vol_cache","name":"cache","target":"/cache","enabled":true}]}
	}`)); err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker.WorkspaceVolume != volume {
		t.Fatalf("workspace volume override = %q", opts.Docker.WorkspaceVolume)
	}
	if volumeMountAt(t, opts.Docker.Mounts, "/cache").Source != "cache" {
		t.Fatalf("extra mount missing: %+v", opts.Docker.Mounts)
	}
}

func TestPromptSandboxOptionsLocalIgnoresVolumes(t *testing.T) {
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
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{
		"sandbox":{"volumes":[{"id":"vol_cache","name":"cache","target":"/cache","enabled":true}]}
	}`)); err != nil {
		t.Fatal(err)
	}

	ag := New(runtime.NewStore(), store, sandboxconfig.Engine{DataDir: dataDir})
	opts, err := ag.promptSandboxOptions(ctx, runtime.Session{ID: "sess-1", ThreadID: thread.ID})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker != nil {
		t.Fatalf("local kind should ignore docker volumes: %+v", opts.Docker)
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

func TestPromptSandboxOptionsPathPolicyWhitelist(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	seedFreshPlaneSettings(t, store)
	project, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{
		"sandbox":{"volumes":[
			{"id":"vol_workspace","write":false},
			{"id":"vol_cache","name":"cache","target":"/cache","enabled":true,"whitelisted":true,"read":true,"write":true},
			{"id":"vol_hidden","name":"hidden","target":"/secret","enabled":true,"whitelisted":false,"read":true,"write":true}
		],"extraPaths":[
			{"id":"path_tmp","path":"/tmp","enabled":true,"whitelisted":true,"read":true,"write":true,"exec":false}
		]}
	}`)); err != nil {
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
	}
}

func TestProjectWorkspaceRootJoinsDataDir(t *testing.T) {
	got := sandbox.ProjectWorkspaceRoot(filepath.FromSlash("/data"), "proj_x")
	if got != filepath.Join("/data", "projects", "proj_x", "workspace") {
		t.Fatalf("got %q", got)
	}
}
