package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/mcp/streamable"
)

// TestConnection probes a tool integration without running a full agent turn.
func TestConnection(ctx context.Context, ti catalog.ToolIntegration, secrets catalog.ToolIntegrationSecrets) error {
	pin := PinnedIntegration{
		ID:       ti.ID,
		Name:     ti.Name,
		Kind:     ti.Kind,
		Endpoint: ti.Endpoint,
		Mode:     ti.Mode,
		Secrets:  secrets,
		Config:   ti.Config,
	}
	reg := &Registry{
		MCPFactory: func(endpoint string, headers http.Header) MCPClient {
			return &streamableAdapter{Client: &streamable.Client{Endpoint: endpoint, Headers: headers}}
		},
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	switch ti.Kind {
	case catalog.KindSearXNG:
		_, err := reg.searxngSearch(ctx, pin, "agent-fabric-healthcheck", 1)
		return err
	case catalog.KindGetMD, catalog.KindCrawl4AI:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ti.Endpoint, "/")+"/health", nil)
		if err != nil {
			return err
		}
		resp, err := reg.httpClient().Do(req)
		if err != nil {
			// Fallback: POST empty convert may also prove liveness; health is preferred.
			return fmt.Errorf("health check: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			return nil
		}
		return fmt.Errorf("health status %d", resp.StatusCode)
	case catalog.KindLinkup:
		headers := http.Header{}
		if key := strings.TrimSpace(secrets["apiKey"]); key != "" {
			headers.Set("Authorization", "Bearer "+key)
		}
		client := reg.MCPFactory(ti.Endpoint, headers)
		defer func() { _ = client.Close() }()
		if err := client.Initialize(ctx); err != nil {
			return err
		}
		_, err := client.ListTools(ctx)
		return err
	default:
		return fmt.Errorf("unknown kind %q", ti.Kind)
	}
}

type streamableAdapter struct {
	*streamable.Client
}

func (a *streamableAdapter) ListTools(ctx context.Context) ([]MCPTool, error) {
	tools, err := a.Client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MCPTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, MCPTool{Name: t.Name, Description: t.Description})
	}
	return out, nil
}

// Ensure streamableAdapter implements CallTool via embedding.
var _ MCPClient = (*streamableAdapter)(nil)

// Silence unused import if CallTool signature needs json.
var _ = json.RawMessage{}
