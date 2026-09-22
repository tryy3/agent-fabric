package catalog_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestInsertListCheckpoints(t *testing.T) {
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

	created, err := store.InsertCheckpoint(ctx, project.ID, "abc1234deadbeef", "before rewrite", &thread.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ID, "chk_") || created.SHA != "abc1234deadbeef" || created.Label != "before rewrite" {
		t.Fatalf("created %+v", created)
	}
	if created.ThreadID == nil || *created.ThreadID != thread.ID {
		t.Fatalf("threadId %+v", created.ThreadID)
	}

	list, err := store.ListCheckpoints(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list %+v", list)
	}

	if _, err := store.InsertCheckpoint(ctx, "proj_missing", "sha", "x", nil, nil); err == nil {
		t.Fatal("expected missing project")
	}
}
