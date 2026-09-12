package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dataDir := flag.String("data-dir", "./data", "catalog JSON directory")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cat, err := catalog.Open(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	store := runtime.NewStore()
	srv := server.New(*addr, store, cat)
	slog.Info("controlplane listening", "addr", *addr, "data_dir", *dataDir, "acp", "/acp", "catalog", "/v1")
	log.Fatal(srv.ListenAndServe())
}
