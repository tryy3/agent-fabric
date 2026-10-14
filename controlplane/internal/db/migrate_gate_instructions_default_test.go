package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

const defaultGateInstructions = `# Gate instructions

## The agent MUST NOT

- Change anything in production. Production changes go through Git and CD.
- Push directly to the main branch. All changes go through a pull request.
- Install software outside the project's declared dependencies.
- Send data to external services the user has not approved.
- Run irreversible operations against shared systems, such as deletions, cleanup and resets.

## The agent MAY

- Inspect any environment read-only.
- Run tests, lint and builds locally.
- Create branches and push feature branches.
- Read documentation from public sources.`

// planeAt22 returns a database migrated to 22 holding a plane_settings row
// whose gate_instructions is text.
func planeAt22(t *testing.T, text string) (context.Context, string, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.MigrateTo(ctx, url, 22); err != nil {
		t.Fatalf("migrate to 22: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
INSERT INTO plane_settings (id, sandbox, environment, gate_instructions, created_at, updated_at)
VALUES ('default', '{}', '{}', $1, now(), now())`, text); err != nil {
		t.Fatalf("insert plane settings: %v", err)
	}
	return ctx, url, pool
}

func planeGate(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT gate_instructions FROM plane_settings WHERE id = 'default'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

// insertBare creates the plane row the way the catalog does, without naming
// gate_instructions.
func insertBare(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DELETE FROM plane_settings`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO plane_settings (id, sandbox, environment, created_at, updated_at)
VALUES ('default', '{}', '{}', now(), now())`); err != nil {
		t.Fatalf("insert plane settings: %v", err)
	}
}

func TestMigrateGateInstructionsDefaultUp(t *testing.T) {
	t.Run("empty row gets the default", func(t *testing.T) {
		ctx, url, pool := planeAt22(t, "")
		if err := db.Migrate(ctx, url); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		if got := planeGate(t, ctx, pool); got != defaultGateInstructions {
			t.Fatalf("existing empty row = %q, want the default text", got)
		}
	})
	t.Run("user text is unchanged", func(t *testing.T) {
		ctx, url, pool := planeAt22(t, "Never push to main.")
		if err := db.Migrate(ctx, url); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		if got := planeGate(t, ctx, pool); got != "Never push to main." {
			t.Fatalf("user text = %q, want it unchanged", got)
		}
	})
	t.Run("new row gets the default and clearing it sticks", func(t *testing.T) {
		ctx, url, pool := planeAt22(t, "")
		if err := db.Migrate(ctx, url); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		insertBare(t, ctx, pool)
		if got := planeGate(t, ctx, pool); got != defaultGateInstructions {
			t.Fatalf("new row = %q, want the default text", got)
		}
		if _, err := pool.Exec(ctx, `UPDATE plane_settings SET gate_instructions = ''`); err != nil {
			t.Fatal(err)
		}
		if got := planeGate(t, ctx, pool); got != "" {
			t.Fatalf("cleared row = %q, want empty", got)
		}
	})
}

func TestMigrateGateInstructionsDefaultDown(t *testing.T) {
	runDown := func(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
		t.Helper()
		// The embedded migrations are up files only, so the down file runs as plain SQL.
		down, err := os.ReadFile("migrations/00023_gate_instructions_default.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(down)); err != nil {
			t.Fatalf("migrate down: %v", err)
		}
	}
	t.Run("the default text is removed", func(t *testing.T) {
		ctx, url, pool := planeAt22(t, "")
		if err := db.Migrate(ctx, url); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		runDown(t, ctx, pool)
		if got := planeGate(t, ctx, pool); got != "" {
			t.Fatalf("row holding the default after down = %q, want empty", got)
		}
		insertBare(t, ctx, pool)
		if got := planeGate(t, ctx, pool); got != "" {
			t.Fatalf("new row after down = %q, want empty", got)
		}
	})
	t.Run("user text is unchanged", func(t *testing.T) {
		ctx, url, pool := planeAt22(t, "Edited "+strings.TrimPrefix(defaultGateInstructions, "# "))
		if err := db.Migrate(ctx, url); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		want := planeGate(t, ctx, pool)
		runDown(t, ctx, pool)
		if got := planeGate(t, ctx, pool); got != want {
			t.Fatalf("user text after down = %q, want %q", got, want)
		}
	})
}
