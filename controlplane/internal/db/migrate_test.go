package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestMigrateCreatesCatalogTables(t *testing.T) {
	pool := dbtest.Open(t)
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN ('providers', 'agents', 'projects', 'resources', 'plane_settings')
	`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 tables, got %d", n)
	}
	var environments int
	err = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'environments'
	`).Scan(&environments)
	if err != nil {
		t.Fatal(err)
	}
	if environments != 0 {
		t.Fatalf("environments table still present")
	}
}

func TestMigrateBackfillsPersonalProject(t *testing.T) {
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.MigrateTo(ctx, url, 5); err != nil {
		t.Fatalf("migrate to 5: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
INSERT INTO threads (id, title, title_source, created_at, updated_at)
VALUES ('th_old', 'Old chat', 'auto', now(), now())`); err != nil {
		t.Fatalf("insert pre-projects thread: %v", err)
	}

	var projectsBefore int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = 'public' AND table_name = 'projects'`).Scan(&projectsBefore); err != nil {
		t.Fatal(err)
	}
	if projectsBefore != 0 {
		t.Fatalf("projects table existed before migration 6: %d", projectsBefore)
	}

	if err := db.MigrateTo(ctx, url, 6); err != nil {
		t.Fatalf("migrate to 6: %v", err)
	}

	var name, projectID string
	if err := pool.QueryRow(ctx, `
SELECT p.name, t.project_id
FROM threads t
JOIN projects p ON p.id = t.project_id
WHERE t.id = 'th_old'`).Scan(&name, &projectID); err != nil {
		t.Fatalf("backfill join: %v", err)
	}
	if name != "Personal" {
		t.Fatalf("project name %q", name)
	}
	if !strings.HasPrefix(projectID, "proj_") {
		t.Fatalf("project id %q", projectID)
	}

	if err := db.Migrate(ctx, url); err != nil {
		t.Fatalf("migrate remaining: %v", err)
	}

	if err := pool.QueryRow(ctx, `
SELECT p.name FROM threads t
JOIN projects p ON p.id = t.project_id
WHERE t.id = 'th_old'`).Scan(&name); err != nil {
		t.Fatalf("renamed join: %v", err)
	}
	if name != "Default" {
		t.Fatalf("project name %q", name)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE name = 'Default'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("default count = %d", n)
	}
}

func TestMigrateAddsResources(t *testing.T) {
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.MigrateTo(ctx, url, 9); err != nil {
		t.Fatalf("migrate to 9: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var n int
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM information_schema.tables
WHERE table_schema = 'public' AND table_name = 'resources'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("resources existed before 00010: %d", n)
	}

	if err := db.Migrate(ctx, url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM information_schema.columns
WHERE table_name = 'plane_settings' AND column_name = 'environment'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("environment columns = %d", n)
	}
}
