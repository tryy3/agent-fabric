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
	if len(list) != 1 || list[0].Name != catalog.PersonalProjectName {
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

func TestCreateThreadDefaultsToPersonal(t *testing.T) {
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
