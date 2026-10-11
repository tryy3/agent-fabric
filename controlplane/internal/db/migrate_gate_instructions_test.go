package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestMigrateGateInstructions(t *testing.T) {
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.MigrateTo(ctx, url, 21); err != nil {
		t.Fatalf("migrate to 21: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	columns := func() int {
		var n int
		if err := pool.QueryRow(ctx, `
SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = current_schema() AND column_name = 'gate_instructions'
  AND table_name IN ('plane_settings', 'assistants')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if columns() != 0 {
		t.Fatal("gate_instructions exists before migration 22")
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO assistants (id, name, description, version, settings, created_at, updated_at)
VALUES ('asst_old', 'Old', '', 1, '{}', now(), now())`); err != nil {
		t.Fatalf("insert assistant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO plane_settings (id, sandbox, environment, created_at, updated_at)
VALUES ('default', '{}', '{}', now(), now())`); err != nil {
		t.Fatalf("insert plane settings: %v", err)
	}

	if err := db.MigrateTo(ctx, url, 22); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if n := columns(); n != 2 {
		t.Fatalf("gate_instructions columns after up = %d, want 2", n)
	}
	var assistant *string
	var plane string
	if err := pool.QueryRow(ctx, `SELECT gate_instructions FROM assistants WHERE id = 'asst_old'`).Scan(&assistant); err != nil {
		t.Fatal(err)
	}
	if assistant != nil {
		t.Fatalf("existing assistant gate_instructions = %q, want NULL (inherit the plane default)", *assistant)
	}
	if err := pool.QueryRow(ctx, `SELECT gate_instructions FROM plane_settings WHERE id = 'default'`).Scan(&plane); err != nil {
		t.Fatal(err)
	}
	if plane != "" {
		t.Fatalf("existing plane gate_instructions = %q, want empty", plane)
	}

	// The embedded migrations are up files only, so the down file runs as plain SQL.
	down, err := os.ReadFile("migrations/00022_gate_instructions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if n := columns(); n != 0 {
		t.Fatalf("gate_instructions columns after down = %d, want 0", n)
	}
}
