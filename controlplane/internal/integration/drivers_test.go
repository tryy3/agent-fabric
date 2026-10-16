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

func TestCrawl4AIConvertsViaMarkdownEndpoint(t *testing.T) {
	conv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/md" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			URL string `json:"url"`
			F   string `json:"f"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if !strings.HasPrefix(req.URL, "raw:") || !strings.Contains(req.URL, "Hello") {
			t.Errorf("url = %q, want raw: HTML", req.URL)
		}
		if req.F != "fit" {
			t.Errorf("f = %q", req.F)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"url":      req.URL,
			"filter":   req.F,
			"markdown": "# Hi\n\nHello",
			"success":  true,
		})
	}))
	defer conv.Close()

	reg := &integration.Registry{HTTP: conv.Client()}
	out, err := integration.Crawl4AIForTest(reg, integration.PinnedIntegration{
		ID: "ti_c4a", Kind: catalog.KindCrawl4AI, Endpoint: conv.URL + "/",
	}, httpfetch.Result{
		FinalURL:    "https://example.com/",
		ContentType: "text/html",
		Body:        []byte(`<html><title>Hi &amp; bye</title><body>Hello</body></html>`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Hi & bye" || out.FinalURL != "https://example.com/" || !strings.Contains(out.Markdown, "Hello") {
		t.Fatalf("%+v", out)
	}
}

func TestCrawl4AIHandlesLargeEchoedHTML(t *testing.T) {
	// /md echoes the whole raw: URL; a >2 MiB page must still decode.
	page := "<html><head><title>Big</title></head><body><p>" + strings.Repeat("é<b>x</b>&", 300_000) + "</p></body></html>"
	conv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "\\u003c") {
			t.Errorf("request HTML was JSON HTML-escaped")
		}
		var req struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"url": req.URL, "markdown": "ok", "success": true})
	}))
	defer conv.Close()

	reg := &integration.Registry{HTTP: conv.Client()}
	out, err := integration.Crawl4AIForTest(reg, integration.PinnedIntegration{Kind: catalog.KindCrawl4AI, Endpoint: conv.URL}, httpfetch.Result{
		FinalURL: "https://example.com/", ContentType: "text/html", Body: []byte(page),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Markdown != "ok" || out.Title != "Big" {
		t.Fatalf("%+v", out)
	}
}

func TestCrawl4AIPassesThroughPlainText(t *testing.T) {
	reg := &integration.Registry{} // no converter: plain text must not hit /md
	body := "# Readme\n\n- one\n- two\n"
	out, err := integration.Crawl4AIForTest(reg, integration.PinnedIntegration{Kind: catalog.KindCrawl4AI, Endpoint: "http://127.0.0.1:1"}, httpfetch.Result{
		FinalURL: "https://example.com/README.md", ContentType: "text/markdown; charset=utf-8", Body: []byte(body),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Markdown != body {
		t.Fatalf("markdown = %q", out.Markdown)
	}
}

func TestPageConverterUnreachableRedactsEndpointPassword(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	endpoint := strings.Replace(srv.URL, "http://", "http://user:s3cret@", 1)
	srv.Close()
	_, err := integration.ConvertForTest(&integration.Registry{}, integration.PinnedIntegration{Kind: catalog.KindGetMD, Endpoint: endpoint}, httpfetch.Result{
		FinalURL: "https://example.com/", ContentType: "text/html", Body: []byte("<p>x</p>"),
	})
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v", err)
	}
}

func TestPageConverterUnreachableNamesIntegration(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	endpoint := srv.URL
	srv.Close() // nothing listens any more: connection refused

	fetched := httpfetch.Result{FinalURL: "https://example.com/", ContentType: "text/html", Body: []byte("<p>x</p>")}
	reg := &integration.Registry{}
	cases := []struct {
		kind, want string
		run        func(integration.PinnedIntegration) error
	}{
		{catalog.KindCrawl4AI, `Crawl4AI integration "Local crawl"`, func(p integration.PinnedIntegration) error {
			_, err := integration.Crawl4AIForTest(reg, p, fetched)
			return err
		}},
		{catalog.KindGetMD, `get-md integration "Local crawl"`, func(p integration.PinnedIntegration) error {
			_, err := integration.ConvertForTest(reg, p, fetched)
			return err
		}},
	}
	for _, tc := range cases {
		err := tc.run(integration.PinnedIntegration{Name: "Local crawl", Kind: tc.kind, Endpoint: endpoint})
		if err == nil {
			t.Fatalf("%s: expected error", tc.kind)
		}
		if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), endpoint) {
			t.Fatalf("%s: error %q should name integration and %s", tc.kind, err, endpoint)
		}
	}
}

func TestLinkupSearchViaFakeMCP(t *testing.T) {
	fake := &fakeMCP{
		tools:      []integration.MCPTool{{Name: "search"}},
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
	tools       []integration.MCPTool
	callResult  json.RawMessage
	initialized bool
	called      string
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
