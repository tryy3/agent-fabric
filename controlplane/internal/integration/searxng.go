package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (r *Registry) searxngSearch(ctx context.Context, pin PinnedIntegration, query string, maxResults int) (SearchResponse, error) {
	base := strings.TrimRight(pin.Endpoint, "/")
	u, err := url.Parse(base + "/search")
	if err != nil {
		return SearchResponse{}, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return SearchResponse{}, err
	}
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return SearchResponse{}, transportError(ctx, pin, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return SearchResponse{}, err
	}
	r.capture(ctx, "http", http.MethodGet, u.String(), resp.StatusCode, "", truncateCapture(string(body)), map[string]any{
		"integrationId": pin.ID,
		"transport":     "http",
		"kind":          pin.Kind,
		"capability":    "web_search",
	})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SearchResponse{}, fmt.Errorf("searxng status %d", resp.StatusCode)
	}
	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
			Engine  string `json:"engine"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return SearchResponse{}, fmt.Errorf("decode searxng: %w", err)
	}
	out := SearchResponse{Query: query, Results: make([]SearchResult, 0, maxResults)}
	for _, hit := range parsed.Results {
		if len(out.Results) >= maxResults {
			break
		}
		out.Results = append(out.Results, SearchResult{
			Title:   hit.Title,
			URL:     hit.URL,
			Snippet: hit.Content,
			Source:  hit.Engine,
		})
	}
	return out, nil
}
