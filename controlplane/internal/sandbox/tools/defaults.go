// Package tools registers the plane's default environment and control-plane tools.
package tools

import (
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/askuser"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
)

// Tool origin values for catalog metadata.
const (
	OriginEnvironment  = "environment"
	OriginControlPlane = "control_plane"
	OriginMCP          = "mcp"
)

// CatalogEntry is a metadata-only tool definition for catalog HTTP.
type CatalogEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Requires    Requires        `json:"requires"`
	Origin      string          `json:"origin"`
}

// Requires is the capability gate a tool needs from the execution environment.
type Requires struct {
	FS   bool `json:"fs"`
	Exec bool `json:"exec"`
}

// DefaultRegistry returns a registry with the plane's built-in tools.
func DefaultRegistry() *sandbox.Registry {
	registry := sandbox.NewRegistry()
	for _, tool := range askuser.Tools() {
		registry.Register(tool)
	}
	for _, tool := range file.Tools() {
		registry.Register(tool)
	}
	return registry
}

func originForTool(name string) string {
	switch name {
	case "ask_user":
		return OriginControlPlane
	case "web_search", "fetch_page":
		return OriginMCP
	default:
		return OriginEnvironment
	}
}

// CatalogEntries lists every registered default tool definition (no env filter),
// plus stable plane web tools (session-pinned at runtime; always listed for Gate).
func CatalogEntries() ([]CatalogEntry, error) {
	registry := DefaultRegistry()
	all := registry.All()
	webMeta := webCatalogEntries()
	out := make([]CatalogEntry, 0, len(all)+len(webMeta))
	for _, tool := range all {
		entry, err := catalogEntry(tool)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	out = append(out, webMeta...)
	return out, nil
}

func catalogEntry(tool sandbox.Tool) (CatalogEntry, error) {
	params, err := json.Marshal(tool.Parameters)
	if err != nil {
		return CatalogEntry{}, fmt.Errorf("encode tool %q parameters: %w", tool.Name, err)
	}
	return CatalogEntry{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  params,
		Requires: Requires{
			FS:   tool.Requires.FS,
			Exec: tool.Requires.Exec,
		},
		Origin: originForTool(tool.Name),
	}, nil
}

// webCatalogEntries are metadata-only; runtime registration lives in tools/web
// (session-pinned). Kept here to avoid an import cycle through integration/catalog.
func webCatalogEntries() []CatalogEntry {
	return []CatalogEntry{
		{
			Name: "web_search",
			Description: "Search the public web. Returns a short list of title, URL, and snippet results. " +
				"Use for discovery; then fetch_page on selected URLs.",
			Parameters: mustJSON(sandbox.Parameters{
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
			}),
			Requires: Requires{},
			Origin:   OriginMCP,
		},
		{
			Name: "fetch_page",
			Description: "Fetch one public http(s) page and return cleaned Markdown plus metadata. " +
				"Private and local network URLs are rejected.",
			Parameters: mustJSON(sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"url": {
						Type:        "string",
						Description: "Absolute http or https URL to read.",
					},
				},
				Required: []string{"url"},
			}),
			Requires: Requires{},
			Origin:   OriginMCP,
		},
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
