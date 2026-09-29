// Package integration resolves plane-owned web tool capabilities to drivers.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/httpfetch"
)

const (
	DefaultMaxResults = 5
	MaxMaxResults     = 10
	MaxQueryLen       = 512
	MaxMarkdownBytes  = 32 << 10 // 32 KiB model-visible
)

// SearchResult is one stable web_search hit.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source,omitempty"`
}

// SearchResponse is the stable web_search tool result.
type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

// PageResponse is the stable fetch_page tool result.
type PageResponse struct {
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl"`
	Title       string `json:"title"`
	ContentType string `json:"contentType"`
	Markdown    string `json:"markdown"`
	Truncated   bool   `json:"truncated,omitempty"`
}

// PinnedIntegration is the session-pinned driver config.
type PinnedIntegration struct {
	ID       string
	Name     string
	Kind     string
	Endpoint string
	Mode     string
	Secrets  catalog.ToolIntegrationSecrets
	Config   json.RawMessage
}

// WebPin holds resolved web tool bindings for a session.
type WebPin struct {
	WebSearch *PinnedIntegration // nil = disabled / omitted
	FetchPage *PinnedIntegration
}

// CaptureHook records a scrubbed HTTP/MCP hop (optional).
type CaptureHook func(ctx context.Context, hopKind, method, url string, status int, reqBody, respBody string, meta map[string]any)

// Registry dispatches stable capabilities to kind-specific drivers.
type Registry struct {
	Fetcher *httpfetch.Client
	HTTP    *http.Client
	Capture CaptureHook
	// MCPFactory creates Streamable HTTP MCP clients (Linkup).
	MCPFactory func(endpoint string, headers http.Header) MCPClient
}

// MCPClient is the narrow MCP surface used by Linkup.
type MCPClient interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]MCPTool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error)
	Close() error
}

// MCPTool is a discovered MCP tool name.
type MCPTool struct {
	Name        string
	Description string
}

func (r *Registry) httpClient() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (r *Registry) fetcher() *httpfetch.Client {
	if r.Fetcher != nil {
		return r.Fetcher
	}
	return &httpfetch.Client{}
}

func (r *Registry) capture(ctx context.Context, hopKind, method, url string, status int, reqBody, respBody string, meta map[string]any) {
	if r.Capture == nil {
		return
	}
	r.Capture(ctx, hopKind, method, url, status, reqBody, respBody, meta)
}

// WebSearch runs the pinned web_search integration.
func (r *Registry) WebSearch(ctx context.Context, pin PinnedIntegration, query string, maxResults int) (SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > MaxQueryLen {
		return SearchResponse{}, fmt.Errorf("query must be 1–%d characters", MaxQueryLen)
	}
	if maxResults <= 0 {
		maxResults = DefaultMaxResults
	}
	if maxResults > MaxMaxResults {
		maxResults = MaxMaxResults
	}
	switch pin.Kind {
	case catalog.KindSearXNG:
		return r.searxngSearch(ctx, pin, query, maxResults)
	case catalog.KindLinkup:
		return r.linkupSearch(ctx, pin, query, maxResults)
	default:
		return SearchResponse{}, fmt.Errorf("integration kind %q does not support web_search", pin.Kind)
	}
}

// FetchPage runs the pinned fetch_page integration.
func (r *Registry) FetchPage(ctx context.Context, pin PinnedIntegration, pageURL string) (PageResponse, error) {
	switch pin.Kind {
	case catalog.KindLinkup:
		return r.linkupFetch(ctx, pin, pageURL)
	case catalog.KindGetMD, catalog.KindCrawl4AI:
		fetched, err := r.fetcher().Get(ctx, pageURL)
		if err != nil {
			return PageResponse{}, err
		}
		return r.convertFromFetch(ctx, pin, fetched, "/convert")
	default:
		return PageResponse{}, fmt.Errorf("integration kind %q does not support fetch_page", pin.Kind)
	}
}

// TruncateMarkdown limits model-visible markdown to MaxMarkdownBytes.
func TruncateMarkdown(md string) (string, bool) {
	if len(md) <= MaxMarkdownBytes {
		return md, false
	}
	// Cut on a rune boundary.
	cut := MaxMarkdownBytes
	for cut > 0 && !utf8.RuneStart(md[cut]) {
		cut--
	}
	return md[:cut], true
}
