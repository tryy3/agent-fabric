package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/config"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	store := runtime.NewStore()
	catalogStore, err := catalog.Open("./data")
	if err != nil {
		log.Fatal(err)
	}
	streamer := provider.NewOpenAI(cfg.BaseURL, cfg.APIKey, cfg.Model, nil)
	srv := server.New(*addr, store, catalogStore, streamer)
	slog.Info("controlplane listening",
		"addr", *addr,
		"acp", "/acp",
		"catalog", "/v1/",
		"openai_base_url", cfg.BaseURL,
		"openai_model", cfg.Model,
		"openai_api_key_set", cfg.APIKey != "",
		"openai_api_key_prefix", keyPrefix(cfg.APIKey),
	)
	log.Fatal(srv.ListenAndServe())
}

func keyPrefix(key string) string {
	const keep = 12
	if len(key) <= keep {
		return "(short)"
	}
	return key[:keep] + "…"
}
