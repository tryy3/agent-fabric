package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestCreateListProject(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))

	list, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != catalog.DefaultProjectName {
		t.Fatalf("seeded projects = %+v", list)
	}
	if !strings.HasPrefix(list[0].ID, "proj_") {
		t.Fatalf("personal id %q", list[0].ID)
	}
	raw, err := json.Marshal(list[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"isolation"`) || strings.Contains(string(raw), `"environmentId"`) {
		t.Fatalf("project json = %s", raw)
	}
	if !strings.Contains(string(list[0].Settings), `"resourceId"`) {
		t.Fatalf("settings %s", list[0].Settings)
	}
	if string(list[0].Remotes) != "[]" {
		t.Fatalf("remotes %s", list[0].Remotes)
	}

	p, err := store.CreateProject(ctx, "  Landing page  ", "prototype")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.ID, "proj_") || p.Name != "Landing page" || p.Description != "prototype" {
		t.Fatalf("create = %+v", p)
	}
	if !strings.Contains(string(p.Settings), `"resourceId"`) {
		t.Fatalf("create settings missing resourceId: %s", p.Settings)
	}
	resolved, err := store.ResolveEnvironment(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Resource == nil || resolved.ResourceID == nil {
		t.Fatalf("created project has no resource: %+v", resolved)
	}
	var spec struct {
		ContainerName string `json:"containerName"`
		Volumes       []struct {
			Name   string `json:"name"`
			Target string `json:"target"`
		} `json:"volumes"`
	}
	if err := json.Unmarshal(resolved.Resource.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.ContainerName != "agent-fabric-container-"+p.ID {
		t.Fatalf("containerName %q", spec.ContainerName)
	}
	if len(spec.Volumes) != 1 || spec.Volumes[0].Name != "agent-fabric-vol-"+p.ID ||
		spec.Volumes[0].Target != catalog.DefaultWorkspaceRoot {
		t.Fatalf("volumes %+v", spec.Volumes)
	}

	got, err := store.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID || got.Name != "Landing page" {
		t.Fatalf("get = %+v", got)
	}

	_, err = store.GetProject(ctx, "proj_missing")
	if err == nil || !errors.Is(err, catalog.ErrProjectNotFound) {
		t.Fatalf("missing: %v", err)
	}

	_, err = store.CreateProject(ctx, "  ", "")
	if err == nil {
		t.Fatal("expected empty name error")
	}
}

func TestUpdateAndDeleteProject(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProject(ctx, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	desc := "notes"
	updated, err := store.UpdateProject(ctx, p.ID, &name, &desc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed" || updated.Description != "notes" {
		t.Fatalf("update = %+v", updated)
	}

	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	_, err = store.GetProject(ctx, p.ID)
	if !errors.Is(err, catalog.ErrProjectNotFound) {
		t.Fatalf("deleted get: %v", err)
	}
}

func TestDeleteProjectRemovesThreadsMessagesAndCheckpoints(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, err := store.CreateProject(ctx, "Busy", "")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThreadForProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitTurn(ctx, th.ID, "hello", catalog.AssistantTurn{Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	threadID := th.ID
	if _, err := store.InsertCheckpoint(ctx, p.ID, "abc123", "before delete", &threadID, nil); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetProject(ctx, p.ID); !errors.Is(err, catalog.ErrProjectNotFound) {
		t.Fatalf("project: %v", err)
	}
	if _, err := store.GetThread(ctx, th.ID); !errors.Is(err, catalog.ErrThreadNotFound) {
		t.Fatalf("thread: %v", err)
	}
	var messages, checkpoints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM messages`).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_checkpoints`).Scan(&checkpoints); err != nil {
		t.Fatal(err)
	}
	if messages != 0 || checkpoints != 0 {
		t.Fatalf("messages=%d checkpoints=%d", messages, checkpoints)
	}
}

func TestDeleteProjectLeavesResource(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	spec := json.RawMessage(`{"image":"alpine:3.20","containerName":"box","volumes":[{"id":"vol_0123456789abcdef","enabled":true,"name":"disk","target":"/workspace","whitelisted":true,"read":true,"write":true,"exec":true}]}`)
	res, err := store.CreateResource(ctx, "Box", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.CreateProject(ctx, "Shared work", "")
	if err != nil {
		t.Fatal(err)
	}
	link, err := json.Marshal(map[string]any{"environment": map[string]string{"resourceId": res.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, p.ID, nil, nil, link); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	resources, err := store.ListResources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range resources {
		if row.ID == res.ID {
			return
		}
	}
	t.Fatalf("resource %q missing after project delete", res.ID)
}

func TestCreateThreadDefaultsToDefault(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	personal, err := store.ListProjects(ctx)
	if err != nil || len(personal) == 0 {
		t.Fatalf("projects: %v %+v", err, personal)
	}
	if th.ProjectID != personal[0].ID {
		t.Fatalf("thread project %q want %q", th.ProjectID, personal[0].ID)
	}
}

func TestCreateAndListThreadsByProject(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	landing, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	inLanding, err := store.CreateThreadForProject(ctx, landing.ID)
	if err != nil {
		t.Fatal(err)
	}
	inPersonal, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}

	filtered, err := store.ListThreads(ctx, landing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].ID != inLanding.ID {
		t.Fatalf("filtered = %+v", filtered)
	}

	all, err := store.ListThreads(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %+v", all)
	}
	ids := map[string]bool{all[0].ID: true, all[1].ID: true}
	if !ids[inLanding.ID] || !ids[inPersonal.ID] {
		t.Fatalf("all missing threads: %+v", all)
	}

	_, err = store.CreateThreadForProject(ctx, "proj_missing")
	if !errors.Is(err, catalog.ErrProjectNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

func TestProjectJSONRoundTripSettings(t *testing.T) {
	p := catalog.Project{
		ID:       "proj_x",
		Name:     "N",
		Settings: json.RawMessage(`{}`),
		Remotes:  json.RawMessage(`[]`),
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"settings":{}`) {
		t.Fatalf("json = %s", b)
	}
	if !strings.Contains(string(b), `"remotes":[]`) {
		t.Fatalf("json = %s", b)
	}
	if strings.Contains(string(b), `"isolation"`) || strings.Contains(string(b), `"environmentId"`) {
		t.Fatalf("json = %s", b)
	}
}

func TestUpdateProjectMergesSettingsGroupsAndRemotes(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.UpdateProject(ctx, p.ID, nil, nil, json.RawMessage(`{
		"sandbox":{"image":"alpine:3.20","containerName":"box-{projectID}"},
		"allowedAgents":["agent_1"],
		"tools":{"allow":["read_file"]},
		"mcp":{"servers":[]},
		"memory":{"enabled":false},
		"context":{"items":[]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpdateProject(ctx, p.ID, nil, nil, json.RawMessage(`{
		"sandbox":{"kind":"local"},
		"allowedAgents":["agent_2"],
		"mcp":{"notes":"stub"}
	}`), json.RawMessage(`[{"id":"rmt_1","kind":"github","urlOrBucket":"https://github.com/acme/landing"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(second.Settings), `"alpine:3.20"`) || !strings.Contains(string(second.Settings), `"local"`) {
		t.Fatalf("sandbox clobbered: %s", second.Settings)
	}
	if !strings.Contains(string(second.Settings), `"agent_2"`) || strings.Contains(string(second.Settings), `"agent_1"`) {
		t.Fatalf("allowedAgents = %s", second.Settings)
	}
	if !strings.Contains(string(second.Settings), `"read_file"`) || !strings.Contains(string(second.Settings), `"stub"`) {
		t.Fatalf("nested groups lost: first=%s second=%s", first.Settings, second.Settings)
	}
	if !strings.Contains(string(second.Remotes), `"rmt_1"`) || !strings.Contains(string(second.Remotes), `"github"`) {
		t.Fatalf("remotes = %s", second.Remotes)
	}

	resolved, err := store.ResolvedProjectSandbox(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Kind == nil || *resolved.Kind != "local" {
		t.Fatalf("resolved kind = %v", resolved.Kind)
	}
	if resolved.Image == nil || *resolved.Image != "alpine:3.20" {
		t.Fatalf("resolved image = %v", resolved.Image)
	}
	if resolved.ContainerName == nil || !strings.Contains(*resolved.ContainerName, p.ID) {
		t.Fatalf("resolved container = %v", resolved.ContainerName)
	}
}

func TestDefaultProjectCannotBeDeletedOrRenamed(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	list, err := store.ListProjects(ctx)
	if err != nil || len(list) != 1 || list[0].Name != catalog.DefaultProjectName {
		t.Fatalf("seeded: %v %+v", err, list)
	}
	def := list[0]

	if err := store.DeleteProject(ctx, def.ID); !errors.Is(err, catalog.ErrDefaultProject) {
		t.Fatalf("delete default: %v", err)
	}
	if _, err := store.GetProject(ctx, def.ID); err != nil {
		t.Fatal(err)
	}

	other, err := store.CreateProject(ctx, catalog.DefaultProjectName, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteProject(ctx, other.ID); !errors.Is(err, catalog.ErrDefaultProject) {
		t.Fatalf("delete second default: %v", err)
	}

	renamed := "Renamed"
	if _, err := store.UpdateProject(ctx, def.ID, &renamed, nil, nil); !errors.Is(err, catalog.ErrDefaultProjectRename) {
		t.Fatalf("rename: %v", err)
	}
	desc := "kept"
	updated, err := store.UpdateProject(ctx, def.ID, nil, &desc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != catalog.DefaultProjectName || updated.Description != "kept" {
		t.Fatalf("description patch = %+v", updated)
	}
	same := catalog.DefaultProjectName
	updated, err = store.UpdateProject(ctx, def.ID, &same, nil, nil)
	if err != nil || updated.Name != catalog.DefaultProjectName {
		t.Fatalf("same name: %v %+v", err, updated)
	}
}
