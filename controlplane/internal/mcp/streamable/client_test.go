package streamable_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryy3/agent-fabric/internal/mcp/streamable"
)

func TestStreamableInitializeListCall(t *testing.T) {
	var gotSession string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			ID     int64           `json:"id"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-1")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"protocolVersion": "2024-11-05"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			gotSession = r.Header.Get("Mcp-Session-Id")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"tools": []map[string]any{{"name": "search", "description": "Search"}}},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"results": []map[string]any{{"title": "T", "url": "https://t.test"}}},
			})
		default:
			http.Error(w, "unknown method", 400)
		}
	}))
	defer srv.Close()

	c := &streamable.Client{Endpoint: srv.URL, HTTP: srv.Client()}
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotSession != "sess-1" || len(tools) != 1 || tools[0].Name != "search" {
		t.Fatalf("session=%q tools=%v", gotSession, tools)
	}
	raw, err := c.CallTool(context.Background(), "search", map[string]any{"query": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !stringsContains(string(raw), "t.test") {
		t.Fatalf("result = %s", raw)
	}
	_ = c.Close()
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
