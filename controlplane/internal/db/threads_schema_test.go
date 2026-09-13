package db_test

import (
	"context"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestThreadsSchemaAllowsUntitledInsert(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO threads (id, title, title_source, created_at, updated_at)
VALUES ('th_test', 'Untitled', 'auto', now(), now())`)
	if err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM threads`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}
