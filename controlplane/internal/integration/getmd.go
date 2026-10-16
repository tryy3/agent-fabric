package integration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/httpfetch"
)

// converterMaxResponse bounds a get-md /convert response (markdown only).
const converterMaxResponse = 2 << 20

// convertFromFetch converts plane-fetched bytes through get-md's POST /convert.
func (r *Registry) convertFromFetch(ctx context.Context, pin PinnedIntegration, fetched httpfetch.Result) (PageResponse, error) {
	respBody, err := r.postConverter(ctx, pin, "/convert", map[string]any{
		"url":         fetched.FinalURL,
		"contentType": fetched.ContentType,
		"html":        string(fetched.Body),
	}, converterMaxResponse)
	if err != nil {
		return PageResponse{}, err
	}
	var parsed struct {
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
		URL      string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return PageResponse{}, fmt.Errorf("%s: decode /convert response: %w", integrationLabel(pin), err)
	}
	md, truncated := TruncateMarkdown(parsed.Markdown)
	outURL := parsed.URL
	if outURL == "" {
		outURL = fetched.FinalURL
	}
	return PageResponse{
		URL:         fetched.FinalURL,
		FinalURL:    outURL,
		Title:       parsed.Title,
		ContentType: fetched.ContentType,
		Markdown:    md,
		Truncated:   truncated,
	}, nil
}
