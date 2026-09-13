package db_test

import (
	"context"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestMigrateCreatesCatalogTables(t *testing.T) {
	pool := dbtest.Open(t)
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN ('providers', 'agents')
	`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 tables, got %d", n)
	}
}
