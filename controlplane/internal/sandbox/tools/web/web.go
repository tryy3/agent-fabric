// Package web registers plane-owned web_search and fetch_page tools.
package web

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/integration"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

const (
	SearchName = "web_search"
	FetchName  = "fetch_page"
)

// Runner executes pinned web tools.
type Runner struct {
	Registry *integration.Registry
	Pin      integration.WebPin
}

// Register adds enabled web tools to registry.
func (r *Runner) Register(reg *sandbox.Registry) {
	if r == nil || r.Registry == nil {
		return
	}
	if r.Pin.WebSearch != nil {
		pin := *r.Pin.WebSearch
		reg.Register(sandbox.Tool{
			Name: SearchName,
			Description: "Search the public web. Returns a short list of title, URL, and snippet results. " +
				"Use for discovery; then fetch_page on selected URLs.",
			Parameters: sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"query": {
						Type:        "string",
						Description: "Search query (1–512 characters).",
					},
					"max_results": {
						Type:        "integer",
						Description: "Maximum results to return (default 5, max 10).",
					},
				},
				Required: []string{"query"},
			},
			Requires: sandbox.Capabilities{},
			Run: func(ctx context.Context, _ sandbox.Environment, args json.RawMessage) (string, error) {
				return r.runSearch(ctx, pin, args)
			},
		})
	}
	if r.Pin.FetchPage != nil {
		pin := *r.Pin.FetchPage
		reg.Register(sandbox.Tool{
			Name: FetchName,
			Description: "Fetch one public http(s) page and return cleaned Markdown plus metadata. " +
				"Private and local network URLs are rejected.",
			Parameters: sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"url": {
						Type:        "string",
						Description: "Absolute http or https URL to read.",
					},
				},
				Required: []string{"url"},
			},
			Requires: sandbox.Capabilities{},
			Run: func(ctx context.Context, _ sandbox.Environment, args json.RawMessage) (string, error) {
				return r.runFetch(ctx, pin, args)
			},
		})
	}
}

func (r *Runner) runSearch(ctx context.Context, pin integration.PinnedIntegration, raw json.RawMessage) (string, error) {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("decode web_search arguments: %w", err)
	}
	out, err := r.Registry.WebSearch(ctx, pin, args.Query, args.MaxResults)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *Runner) runFetch(ctx context.Context, pin integration.PinnedIntegration, raw json.RawMessage) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("decode fetch_page arguments: %w", err)
	}
	out, err := r.Registry.FetchPage(ctx, pin, args.URL)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
