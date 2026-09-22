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
	if list[0].Isolation != catalog.IsolationIsolated {
		t.Fatalf("isolation %q", list[0].Isolation)
	}
	if string(list[0].Settings) != "{}" {
		t.Fatalf("settings %s", list[0].Settings)
	}
	if string(list[0].Remotes) != "[]" {
		t.Fatalf("remotes %s", list[0].Remotes)
	}

	p, err := store.CreateProject(ctx, "  Landing page  ", "prototype", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.ID, "proj_") || p.Name != "Landing page" || p.Description != "prototype" {
		t.Fatalf("create = %+v", p)
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

	_, err = store.CreateProject(ctx, "  ", "", "")
	if err == nil {
		t.Fatal("expected empty name error")
	}
}

func TestUpdateAndDeleteProject(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProject(ctx, "Draft", "", catalog.IsolationIsolated)
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	desc := "notes"
	updated, err := store.UpdateProject(ctx, p.ID, &name, &desc, nil, nil)
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

func TestDeleteProjectInUse(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProject(ctx, "Busy", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateThreadForProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteProject(ctx, p.ID); !errors.Is(err, catalog.ErrProjectInUse) {
		t.Fatalf("delete in use: %v", err)
	}
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
	landing, err := store.CreateProject(ctx, "Landing", "", "")
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

func TestCreateProjectRejectsUnknownIsolation(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	_, err := store.CreateProject(ctx, "X", "", "ssh")
	if err == nil || !strings.Contains(err.Error(), "unknown isolation") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateSharedProjectUsesEnvironment(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	volume := "agent-fabric.env.custom"
	env, err := store.CreateEnvironment(ctx, "shared-tools", "docker", &volume)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(env.ID, "env_") || env.VolumeName == nil || *env.VolumeName != volume {
		t.Fatalf("environment = %+v", env)
	}

	p, err := store.CreateSharedProject(ctx, "Shared work", "", env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Isolation != catalog.IsolationShared || p.EnvironmentID == nil || *p.EnvironmentID != env.ID {
		t.Fatalf("shared project = %+v", p)
	}

	_, err = store.GetEnvironment(ctx, "env_missing")
	if err == nil || !errors.Is(err, catalog.ErrEnvironmentNotFound) {
		t.Fatalf("missing env: %v", err)
	}
	_, err = store.CreateSharedProject(ctx, "Nope", "", "env_missing")
	if err == nil || !errors.Is(err, catalog.ErrEnvironmentNotFound) {
		t.Fatalf("missing shared: %v", err)
	}
}

func TestProjectJSONRoundTripSettings(t *testing.T) {
	p := catalog.Project{
		ID:        "proj_x",
		Name:      "N",
		Isolation: catalog.IsolationIsolated,
		Settings:  json.RawMessage(`{}`),
		Remotes:   json.RawMessage(`[]`),
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
}

func TestUpdateProjectMergesSettingsGroupsAndRemotes(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProject(ctx, "Landing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.UpdateProject(ctx, p.ID, nil, nil, nil, json.RawMessage(`{
		"sandbox":{"image":"alpine:3.20"},
		"allowedAgents":["agent_1"],
		"tools":{"allow":["read_file"]},
		"mcp":{"servers":[]},
		"memory":{"enabled":false},
		"context":{"items":[]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpdateProject(ctx, p.ID, nil, nil, nil, json.RawMessage(`{
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
