package dbtest

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tryy3/agent-fabric/internal/appmigrate"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
)

const (
	pgUser = "agent"
	pgDB   = "agentfabric"
)

func Start(t testing.TB) string {
	t.Helper()
	ctx := context.Background()

	requireBinary(t, "initdb")
	requireBinary(t, "postgres")

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	initCmd := exec.Command("initdb",
		"-D", dataDir,
		"-U", pgUser,
		"--auth-local=trust",
		"--auth-host=trust",
	)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("initdb: %v\n%s", err, out)
	}

	port := pickFreePort(t)
	// Keep socket dir short: t.TempDir() paths can exceed Unix socket limits for long test names.
	socketDir := filepath.Join(os.TempDir(), fmt.Sprintf("pgtest-%d", port))
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatalf("mkdir sockets: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })

	logFile, err := os.Create(filepath.Join(tmpDir, "postgres.log"))
	if err != nil {
		t.Fatalf("create postgres log: %v", err)
	}
	t.Cleanup(func() { _ = logFile.Close() })

	pgCmd := exec.Command("postgres",
		"-D", dataDir,
		"-k", socketDir,
		"-c", "listen_addresses=127.0.0.1",
		"-c", fmt.Sprintf("port=%d", port),
	)
	pgCmd.Stdout = logFile
	pgCmd.Stderr = logFile
	if err := pgCmd.Start(); err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if pgCmd.Process != nil {
			_ = pgCmd.Process.Kill()
		}
		_ = pgCmd.Wait()
	})

	adminURL := fmt.Sprintf("postgres://%s@127.0.0.1:%d/postgres?sslmode=disable", pgUser, port)
	waitForPostgres(t, ctx, adminURL)

	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+pgDB); err != nil {
		adminPool.Close()
		t.Fatalf("create database: %v", err)
	}
	adminPool.Close()

	return fmt.Sprintf("postgres://%s@127.0.0.1:%d/%s?sslmode=disable", pgUser, port, pgDB)
}

func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	url := Start(t)
	if err := appmigrate.RunMigrations(ctx, url, "", catalog.DeprecatedSandbox{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func requireBinary(t testing.TB, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Fatalf("%s not found on PATH (install postgresql in dev shell)", name)
	}
}

func pickFreePort(t testing.TB) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return port
}

func waitForPostgres(t testing.TB, ctx context.Context, databaseURL string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			err = pool.Ping(ctx)
			pool.Close()
			if err == nil {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("postgres did not become ready at %s within 30s", databaseURL)
}
