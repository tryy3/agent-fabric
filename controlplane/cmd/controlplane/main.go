package main

import (
	"flag"
	"log"
	"log/slog"

	"github.com/tryy3/agent-fabric/internal/config"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	store := runtime.NewStore()
	streamer := provider.NewOpenAI(cfg.BaseURL, cfg.APIKey, cfg.Model, nil)
	srv := server.New(*addr, store, streamer)
	slog.Info("controlplane listening", "addr", *addr, "acp", "/acp", "model", cfg.Model)
	log.Fatal(srv.ListenAndServe())
}
