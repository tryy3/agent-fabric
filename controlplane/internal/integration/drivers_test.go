package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/httpfetch"
	"github.com/tryy3/agent-fabric/internal/integration"
)

func TestSearXNGSearchNormalized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("format") != "json" {
			t.Fatalf("path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "One", "url": "https://example.com/1", "content": "snippet", "engine": "duckduckgo"},
				{"title": "Two", "url": "https://example.com/2", "content": "more", "engine": "bing"},
			},
		})
	}))
	defer srv.Close()

	reg := &integration.Registry{HTTP: srv.Client()}
	out, err := reg.WebSearch(context.Background(), integration.PinnedIntegration{
		ID: "ti_1", Kind: catalog.KindSearXNG, Endpoint: srv.URL,
	}, "golang", 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Query != "golang" || len(out.Results) != 1 || out.Results[0].Title != "One" {
		t.Fatalf("%+v", out)
	}
}

func TestConvertPageFromFetchedBytes(t *testing.T) {
	conv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/convert" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Hello") {
			t.Fatalf("body = %s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"title":    "Hi",
			"markdown": "# Hi\n\nHello",
			"url":      "https://example.com/",
		})
	}))
	defer conv.Close()

	reg := &integration.Registry{HTTP: conv.Client()}
	out, err := integration.ConvertForTest(reg, integration.PinnedIntegration{
		ID: "ti_md", Kind: catalog.KindGetMD, Endpoint: conv.URL,
	}, httpfetch.Result{
		FinalURL:    "https://example.com/",
		ContentType: "text/html",
		Body:        []byte(`<html><title>Hi</title><body>Hello</body></html>`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Hi" || !strings.Contains(out.Markdown, "Hello") || out.Truncated {
		t.Fatalf("%+v", out)
	}
}

func TestLinkupSearchViaFakeMCP(t *testing.T) {
	fake := &fakeMCP{
		tools: []integration.MCPTool{{Name: "search"}},
		callResult: json.RawMessage(`{"results":[{"title":"A","url":"https://a.test","snippet":"s"}]}`),
	}
	reg := &integration.Registry{
		MCPFactory: func(string, http.Header) integration.MCPClient { return fake },
	}
	out, err := reg.WebSearch(context.Background(), integration.PinnedIntegration{
		ID: "ti_l", Kind: catalog.KindLinkup, Endpoint: "https://mcp.example/mcp",
		Secrets: catalog.ToolIntegrationSecrets{"apiKey": "k"},
	}, "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].Title != "A" {
		t.Fatalf("%+v", out)
	}
	if !fake.initialized || fake.called != "search" {
		t.Fatalf("fake state %+v", fake)
	}
}

func TestLinkupFetchViaFakeMCP(t *testing.T) {
	fake := &fakeMCP{
		tools: []integration.MCPTool{
			{Name: "linkup-search"},
			{Name: "linkup-fetch"},
		},
		callResult: json.RawMessage(`{"markdown":"# Hello\n\nWorld","title":"Hello","url":"https://example.com/"}`),
	}
	reg := &integration.Registry{
		MCPFactory: func(string, http.Header) integration.MCPClient { return fake },
	}
	out, err := reg.FetchPage(context.Background(), integration.PinnedIntegration{
		ID: "ti_l", Kind: catalog.KindLinkup, Endpoint: "https://mcp.example/mcp",
		Secrets: catalog.ToolIntegrationSecrets{"apiKey": "k"},
	}, "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Hello" || !strings.Contains(out.Markdown, "World") {
		t.Fatalf("%+v", out)
	}
	if fake.called != "linkup-fetch" {
		t.Fatalf("called %q", fake.called)
	}
}

func TestLinkupFetchRejectsPrivateURL(t *testing.T) {
	reg := &integration.Registry{
		MCPFactory: func(string, http.Header) integration.MCPClient {
			return &fakeMCP{tools: []integration.MCPTool{{Name: "linkup-fetch"}}}
		},
	}
	_, err := reg.FetchPage(context.Background(), integration.PinnedIntegration{
		Kind: catalog.KindLinkup, Endpoint: "https://mcp.example/mcp",
	}, "http://127.0.0.1/")
	if err == nil {
		t.Fatal("expected SSRF denial before MCP call")
	}
}

func TestWebSearchRejectsEmptyQuery(t *testing.T) {
	reg := &integration.Registry{}
	_, err := reg.WebSearch(context.Background(), integration.PinnedIntegration{Kind: catalog.KindSearXNG}, "  ", 5)
	if err == nil {
		t.Fatal("expected error")
	}
}

type fakeMCP struct {
	tools        []integration.MCPTool
	callResult   json.RawMessage
	initialized  bool
	called       string
}

func (f *fakeMCP) Initialize(context.Context) error { f.initialized = true; return nil }
func (f *fakeMCP) ListTools(context.Context) ([]integration.MCPTool, error) {
	return f.tools, nil
}
func (f *fakeMCP) CallTool(_ context.Context, name string, _ map[string]any) (json.RawMessage, error) {
	f.called = name
	return f.callResult, nil
}
func (f *fakeMCP) Close() error { return nil }
