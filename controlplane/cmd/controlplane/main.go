package main

import (
	"flag"
	"log"
	"log/slog"

	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	store := runtime.NewStore()
	srv := server.New(*addr, store)
	slog.Info("controlplane listening", "addr", *addr, "acp", "/acp")
	log.Fatal(srv.ListenAndServe())
}
