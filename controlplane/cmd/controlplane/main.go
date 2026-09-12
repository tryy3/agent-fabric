package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	databaseURL := os.Getenv("DATABASE_URL")
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
	store := runtime.NewStore()
	srv := server.New(*addr, store, cat)
	slog.Info("controlplane listening", "addr", *addr, "acp", "/acp", "catalog", "/v1")
	log.Fatal(srv.ListenAndServe())
}
