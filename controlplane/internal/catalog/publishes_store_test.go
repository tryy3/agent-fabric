package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestInsertPublishMovesLive(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	in := catalog.PublishInput{Provider: "netlify", Target: "site_1", SiteURL: "https://x"}
	first, err := store.InsertPublish(ctx, project.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{Provider: "internal", SiteURL: "https://y"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.InsertPublish(ctx, project.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Live || !second.Live {
		t.Fatalf("new publishes must be live: %+v %+v", first, second)
	}
	live := map[string]bool{}
	list, err := store.ListPublishes(ctx, project.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		live[p.ID] = p.Live
	}
	if live[first.ID] || !live[second.ID] || !live[other.ID] {
		t.Fatalf("live = %v (first %s, second %s, other provider %s)", live, first.ID, second.ID, other.ID)
	}
}

func syncDeploy(id string, minute int, live bool) catalog.PublishInput {
	return catalog.PublishInput{
		DeployID:  id,
		SiteURL:   "https://demo.netlify.app",
		DeployURL: "https://" + id + "--demo.netlify.app",
		Live:      live,
		CreatedAt: time.Date(2026, 10, 11, 12, minute, 0, 0, time.UTC),
	}
}

func TestReconcilePublishes(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	// Published from agent-fabric: dep_2 (live) and dep_3, which was later
	// deleted on the provider. A row without a deploy id can't be matched.
	for _, id := range []string{"dep_2", "dep_3"} {
		if _, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{
			Provider: "netlify", Target: "site_1", DeployID: id,
			SiteURL: "https://old.netlify.app", DeployURL: "https://" + id + "--old.netlify.app",
		}); err != nil {
			t.Fatal(err)
		}
	}
	unmatched, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{
		Provider: "netlify", Target: "site_1", SiteURL: "https://old.netlify.app",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTarget, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{
		Provider: "netlify", Target: "site_2", DeployID: "dep_x", SiteURL: "https://two.netlify.app",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The provider reports dep_2 (rolled back to, so live) and dep_1, made
	// outside agent-fabric; dep_3 is gone.
	stats, err := store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_2", 2, true),
		syncDeploy("dep_1", 1, false),
		syncDeploy("dep_1", 1, false), // duplicates are ignored
		{DeployID: "dep_nourl"},       // unusable entries are ignored
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (catalog.PublishSyncStats{Added: 1, Removed: 1, Updated: 1}) {
		t.Fatalf("stats = %+v", stats)
	}
	byDeploy := map[string]catalog.Publish{}
	list, err := store.ListPublishes(ctx, project.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		byDeploy[p.DeployID] = p
	}
	if _, ok := byDeploy["dep_3"]; ok {
		t.Fatal("dep_3 should be removed")
	}
	dep1 := byDeploy["dep_1"]
	if dep1.ID == "" || dep1.Live || !dep1.CreatedAt.Equal(time.Date(2026, 10, 11, 12, 1, 0, 0, time.UTC)) ||
		dep1.DeployURL != "https://dep_1--demo.netlify.app" {
		t.Fatalf("imported dep_1 = %+v", dep1)
	}
	if dep2 := byDeploy["dep_2"]; !dep2.Live || dep2.SiteURL != "https://demo.netlify.app" {
		t.Fatalf("dep_2 = %+v", dep2)
	}
	if p := byDeploy[""]; p.ID != unmatched.ID {
		t.Fatalf("row without deploy id must stay: %+v", p)
	}
	if p := byDeploy["dep_x"]; p.ID != otherTarget.ID || !p.Live {
		t.Fatalf("other target must be untouched: %+v", p)
	}

	// A second identical sync changes nothing.
	again, err := store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_2", 2, true), syncDeploy("dep_1", 1, false),
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if again != (catalog.PublishSyncStats{}) {
		t.Fatalf("second sync = %+v", again)
	}
}

func TestReconcilePublishesIncompleteKeepsOlderRows(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_3", 3, false), syncDeploy("dep_2", 2, false), syncDeploy("dep_1", 1, true),
	}, true); err != nil {
		t.Fatal(err)
	}
	// Truncated list that only reaches back to minute 2: dep_1 is outside the
	// window and stays, but is no longer live; dep_3 is gone.
	stats, err := store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_4", 4, true), syncDeploy("dep_2", 2, false),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (catalog.PublishSyncStats{Added: 1, Removed: 1, Updated: 1}) {
		t.Fatalf("stats = %+v", stats)
	}
	list, err := store.ListPublishes(ctx, project.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range list {
		got[p.DeployID] = p.Live
	}
	if len(got) != 3 || !got["dep_4"] || got["dep_2"] || got["dep_1"] {
		t.Fatalf("after sync = %v", got)
	}
}

func TestReconcilePublishesIncompleteIgnoresRolledBackLiveAge(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_5", 5, false), syncDeploy("dep_4", 4, false),
		syncDeploy("dep_3", 3, false), syncDeploy("dep_1", 1, true),
	}, true); err != nil {
		t.Fatal(err)
	}
	// The truncated list reaches back to minute 4; dep_1 is live after a
	// rollback and reported on its own. dep_3 is older than the window, so it
	// stays even though it is newer than dep_1.
	stats, err := store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_5", 5, false), syncDeploy("dep_4", 4, false), syncDeploy("dep_1", 1, true),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (catalog.PublishSyncStats{}) {
		t.Fatalf("stats = %+v", stats)
	}

	// A truncated list with no window at all (only the live deploy) removes
	// nothing.
	stats, err = store.ReconcilePublishes(ctx, project.ID, "netlify", "site_1", []catalog.PublishInput{
		syncDeploy("dep_1", 1, true),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (catalog.PublishSyncStats{}) {
		t.Fatalf("stats without window = %+v", stats)
	}
	list, err := store.ListPublishes(ctx, project.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("got %d publishes, want 4", len(list))
	}
}

func TestGetDeletePublishScopedToProject(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateProject(ctx, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{Provider: "netlify", SiteURL: "https://x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPublish(ctx, other.ID, p.ID); !errors.Is(err, catalog.ErrPublishNotFound) {
		t.Fatalf("cross-project get err = %v", err)
	}
	if err := store.DeletePublish(ctx, other.ID, p.ID); !errors.Is(err, catalog.ErrPublishNotFound) {
		t.Fatalf("cross-project delete err = %v", err)
	}
	if err := store.DeletePublish(ctx, project.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPublish(ctx, project.ID, p.ID); !errors.Is(err, catalog.ErrPublishNotFound) {
		t.Fatalf("after delete err = %v", err)
	}
}
