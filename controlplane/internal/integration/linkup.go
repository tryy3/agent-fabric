package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func (r *Registry) linkupSearch(ctx context.Context, pin PinnedIntegration, query string, maxResults int) (SearchResponse, error) {
	if r.MCPFactory == nil {
		return SearchResponse{}, fmt.Errorf("mcp client factory is not configured")
	}
	headers := http.Header{}
	if key := strings.TrimSpace(pin.Secrets["apiKey"]); key != "" {
		headers.Set("Authorization", "Bearer "+key)
	}
	client := r.MCPFactory(pin.Endpoint, headers)
	defer func() { _ = client.Close() }()

	if err := client.Initialize(ctx); err != nil {
		return SearchResponse{}, fmt.Errorf("linkup initialize: %w", err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("linkup list tools: %w", err)
	}
	toolName := pickSearchTool(tools)
	if toolName == "" {
		return SearchResponse{}, fmt.Errorf("linkup: no search tool discovered")
	}
	args := map[string]any{
		"query": query,
		"depth": "standard",
	}
	raw, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("linkup call: %w", err)
	}
	r.capture(ctx, "mcp", "tools/call", pin.Endpoint, 200, fmt.Sprintf(`{"name":%q,"query":%q}`, toolName, query), truncateCapture(string(raw)), map[string]any{
		"integrationId": pin.ID,
		"transport":     "mcp_streamable_http",
		"kind":          catalog.KindLinkup,
		"capability":    "web_search",
		"mcpTool":       toolName,
	})
	return normalizeLinkupResults(query, maxResults, raw)
}

func pickSearchTool(tools []MCPTool) string {
	preferred := []string{"search", "linkup_search", "web_search", "search-web"}
	byName := map[string]string{}
	for _, t := range tools {
		byName[strings.ToLower(t.Name)] = t.Name
	}
	for _, p := range preferred {
		if name, ok := byName[p]; ok {
			return name
		}
	}
	for _, t := range tools {
		if strings.Contains(strings.ToLower(t.Name), "search") {
			return t.Name
		}
	}
	if len(tools) == 1 {
		return tools[0].Name
	}
	return ""
}

func normalizeLinkupResults(query string, maxResults int, raw json.RawMessage) (SearchResponse, error) {
	// Accept several shapes: {results:[...]}, {content:[{type,text}]}, or plain array.
	var asObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObj); err == nil {
		if resultsRaw, ok := asObj["results"]; ok {
			return decodeSearchHits(query, maxResults, resultsRaw)
		}
		if contentRaw, ok := asObj["content"]; ok {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(contentRaw, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
						var nested any
						if err := json.Unmarshal([]byte(b.Text), &nested); err == nil {
							reb, _ := json.Marshal(nested)
							return normalizeLinkupResults(query, maxResults, reb)
						}
					}
				}
			}
		}
	}
	var asArr []json.RawMessage
	if err := json.Unmarshal(raw, &asArr); err == nil {
		return decodeSearchHits(query, maxResults, raw)
	}
	return SearchResponse{}, fmt.Errorf("unrecognized linkup result shape")
}

func decodeSearchHits(query string, maxResults int, raw json.RawMessage) (SearchResponse, error) {
	var hits []struct {
		Title   string `json:"title"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Href    string `json:"href"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
		Content string `json:"content"`
		Text    string `json:"text"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(raw, &hits); err != nil {
		return SearchResponse{}, fmt.Errorf("decode search hits: %w", err)
	}
	out := SearchResponse{Query: query, Results: make([]SearchResult, 0, maxResults)}
	for _, h := range hits {
		if len(out.Results) >= maxResults {
			break
		}
		title := firstNonEmpty(h.Title, h.Name)
		u := firstNonEmpty(h.URL, h.Href, h.Link)
		snippet := firstNonEmpty(h.Snippet, h.Content, h.Text)
		if title == "" && u == "" {
			continue
		}
		out.Results = append(out.Results, SearchResult{
			Title:   title,
			URL:     u,
			Snippet: snippet,
			Source:  h.Source,
		})
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
