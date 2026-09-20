package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", "", "HTTP listen address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	sandboxPath := filepath.Join(cwd, "sandbox.json")
	engine, deprecated, err := sandboxconfig.LoadFile(sandboxPath)
	if err != nil {
		log.Fatalf("sandbox config: %v (run controlplane from a directory that contains sandbox.json)", err)
	}

	listenAddr := *addr
	if listenAddr == "" {
		listenAddr = engine.ListenAddr
	}
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = engine.DatabaseURL
	}

	slog.Info("sandbox engine loaded",
		"path", sandboxPath,
		"dataDir", engine.DataDir,
		"listenAddr", listenAddr,
		"dockerRuntime", engine.Docker.Runtime,
	)

	ctx := context.Background()
	if err := db.Migrate(ctx, databaseURL); err != nil {
		log.Fatal(err)
	}
	pool, err := db.OpenPool(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	cat := catalog.Open(pool)
	if _, err := cat.EnsurePlaneSettings(ctx, deprecated); err != nil {
		log.Fatalf("seed plane settings: %v", err)
	}
	store := runtime.NewStore()
	srv := server.New(listenAddr, store, cat, engine)
	slog.Info("controlplane listening", "addr", listenAddr, "acp", "/acp", "catalog", "/v1")
	log.Fatal(srv.ListenAndServe())
}
