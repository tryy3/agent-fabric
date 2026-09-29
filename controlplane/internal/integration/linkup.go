package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/netsafe"
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

func (r *Registry) linkupFetch(ctx context.Context, pin PinnedIntegration, pageURL string) (PageResponse, error) {
	if _, err := netsafe.ValidateURL(pageURL); err != nil {
		return PageResponse{}, err
	}
	if r.MCPFactory == nil {
		return PageResponse{}, fmt.Errorf("mcp client factory is not configured")
	}
	headers := http.Header{}
	if key := strings.TrimSpace(pin.Secrets["apiKey"]); key != "" {
		headers.Set("Authorization", "Bearer "+key)
	}
	client := r.MCPFactory(pin.Endpoint, headers)
	defer func() { _ = client.Close() }()

	if err := client.Initialize(ctx); err != nil {
		return PageResponse{}, fmt.Errorf("linkup initialize: %w", err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return PageResponse{}, fmt.Errorf("linkup list tools: %w", err)
	}
	toolName := pickFetchTool(tools)
	if toolName == "" {
		return PageResponse{}, fmt.Errorf("linkup: no fetch tool discovered")
	}
	args := map[string]any{
		"url": pageURL,
	}
	raw, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return PageResponse{}, fmt.Errorf("linkup fetch: %w", err)
	}
	r.capture(ctx, "mcp", "tools/call", pin.Endpoint, 200, fmt.Sprintf(`{"name":%q,"url":%q}`, toolName, pageURL), truncateCapture(string(raw)), map[string]any{
		"integrationId": pin.ID,
		"transport":     "mcp_streamable_http",
		"kind":          catalog.KindLinkup,
		"capability":    "fetch_page",
		"mcpTool":       toolName,
	})
	return normalizeLinkupPage(pageURL, raw)
}

func pickSearchTool(tools []MCPTool) string {
	preferred := []string{"linkup-search", "search", "linkup_search", "web_search", "search-web"}
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
		lower := strings.ToLower(t.Name)
		if strings.Contains(lower, "search") && !strings.Contains(lower, "research") {
			return t.Name
		}
	}
	return ""
}

func pickFetchTool(tools []MCPTool) string {
	preferred := []string{"linkup-fetch", "fetch", "linkup_fetch", "fetch_page", "fetch-page"}
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
		lower := strings.ToLower(t.Name)
		if strings.Contains(lower, "fetch") {
			return t.Name
		}
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

func normalizeLinkupPage(pageURL string, raw json.RawMessage) (PageResponse, error) {
	markdown, title, finalURL, contentType := extractLinkupPageFields(raw)
	if markdown == "" {
		// MCP content blocks often wrap the payload as text.
		var asObj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &asObj); err == nil {
			if contentRaw, ok := asObj["content"]; ok {
				var blocks []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(contentRaw, &blocks); err == nil {
					for _, b := range blocks {
						if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
							markdown, title, finalURL, contentType = extractLinkupPageFields(json.RawMessage(b.Text))
							if markdown == "" {
								markdown = strings.TrimSpace(b.Text)
							}
							break
						}
					}
				}
			}
		}
	}
	if markdown == "" {
		return PageResponse{}, fmt.Errorf("linkup fetch returned no markdown")
	}
	md, truncated := TruncateMarkdown(markdown)
	if finalURL == "" {
		finalURL = pageURL
	}
	if contentType == "" {
		contentType = "text/markdown"
	}
	return PageResponse{
		URL:         pageURL,
		FinalURL:    finalURL,
		Title:       title,
		ContentType: contentType,
		Markdown:    md,
		Truncated:   truncated,
	}, nil
}

func extractLinkupPageFields(raw json.RawMessage) (markdown, title, finalURL, contentType string) {
	var obj struct {
		Markdown    string `json:"markdown"`
		Content     string `json:"content"`
		Text        string `json:"text"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		FinalURL    string `json:"finalUrl"`
		ContentType string `json:"contentType"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", "", "", ""
	}
	markdown = firstNonEmpty(obj.Markdown, obj.Content, obj.Text)
	title = obj.Title
	finalURL = firstNonEmpty(obj.FinalURL, obj.URL)
	contentType = obj.ContentType
	return markdown, title, finalURL, contentType
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
