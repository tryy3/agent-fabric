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
INSERT INTO threads (id, title, title_source, project_id, created_at, updated_at)
SELECT 'th_test', 'Untitled', 'auto', id, now(), now() FROM projects WHERE name = 'Personal' LIMIT 1`)
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

func TestMessagesPartsColumnDefaultsEmptyArray(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO threads (id, title, title_source, project_id, created_at, updated_at)
SELECT 'th_parts', 'Untitled', 'auto', id, now(), now() FROM projects WHERE name = 'Personal' LIMIT 1`)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO messages (id, thread_id, role, content, position, created_at)
VALUES ('msg_u', 'th_parts', 'user', 'hi', 0, now())`)
	if err != nil {
		t.Fatalf("insert without parts: %v", err)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT parts FROM messages WHERE id='msg_u'`).Scan(&raw); err != nil {
		t.Fatalf("select parts: %v", err)
	}
	if string(raw) != "[]" && string(raw) != "[]\n" {
		t.Fatalf("parts = %s, want []", raw)
	}
}

func TestThreadsViewModeIDColumn(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	var col string
	err := pool.QueryRow(ctx, `
SELECT column_name FROM information_schema.columns
WHERE table_name = 'threads' AND column_name = 'view_mode_id'`).Scan(&col)
	if err != nil {
		t.Fatalf("view_mode_id missing: %v", err)
	}
}
