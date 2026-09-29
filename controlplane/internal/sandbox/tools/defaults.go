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

// CatalogEntries lists every registered default tool definition (no env filter).
func CatalogEntries() ([]CatalogEntry, error) {
	registry := DefaultRegistry()
	all := registry.All()
	out := make([]CatalogEntry, 0, len(all))
	for _, tool := range all {
		params, err := json.Marshal(tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("encode tool %q parameters: %w", tool.Name, err)
		}
		out = append(out, CatalogEntry{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
			Requires: Requires{
				FS:   tool.Requires.FS,
				Exec: tool.Requires.Exec,
			},
			Origin: originForTool(tool.Name),
		})
	}
	return out, nil
}
