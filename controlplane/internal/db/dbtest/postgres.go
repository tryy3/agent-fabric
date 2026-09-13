package dbtest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/tryy3/agent-fabric/internal/db"
)

func configureContainerRuntime(t testing.TB) {
	t.Helper()
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	sock := fmt.Sprintf("/run/user/%d/podman/podman.sock", os.Getuid())
	if _, err := os.Stat(sock); err != nil {
		return
	}
	t.Setenv("DOCKER_HOST", "unix://"+sock)
	t.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", sock)
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	configureContainerRuntime(t)
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("agentfabric"),
		postgres.WithUsername("agent"),
		postgres.WithPassword("agent"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
