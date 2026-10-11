package catalog_test

import (
	"context"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestInsertListPublishes(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{
		Provider:  "netlify",
		Target:    "site_1",
		DeployID:  "dep_1",
		SiteURL:   "https://landing.netlify.app",
		DeployURL: "https://dep_1--landing.netlify.app",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{
		Provider:  "internal",
		SiteURL:   "https://pages.internal/landing",
		DeployURL: "https://pages.internal/landing/v2",
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := store.ListPublishes(ctx, project.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("want newest first, got %+v", list)
	}
	if got := list[1]; got.Provider != "netlify" || got.Target != "site_1" || got.DeployID != "dep_1" ||
		got.SiteURL != "https://landing.netlify.app" || got.DeployURL != "https://dep_1--landing.netlify.app" {
		t.Fatalf("netlify publish = %+v", got)
	}

	netlify, err := store.ListPublishes(ctx, project.ID, "netlify", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(netlify) != 1 || netlify[0].ID != first.ID {
		t.Fatalf("netlify = %+v", netlify)
	}

	limited, err := store.ListPublishes(ctx, project.ID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].ID != second.ID {
		t.Fatalf("limited = %+v", limited)
	}

	if _, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{Provider: "netlify"}); err == nil {
		t.Fatal("expected error without URLs")
	}
	if _, err := store.InsertPublish(ctx, "proj_missing", catalog.PublishInput{Provider: "netlify", SiteURL: "https://x"}); err == nil {
		t.Fatal("expected error for missing project")
	}
	if err := store.DeleteProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
}
