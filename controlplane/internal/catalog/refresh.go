package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

const maxModelsErrorBody = 4 << 10

type modelsListResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

func (s *Store) RefreshModels(ctx context.Context, id string, client *http.Client) (InferenceConnection, error) {
	p, err := s.GetInferenceConnection(ctx, id)
	if err != nil {
		return InferenceConnection{}, err
	}
	if !isKnownConnectionType(p.Type) {
		return InferenceConnection{}, fmt.Errorf("unknown inference connection type %q", p.Type)
	}
	if client == nil {
		client = http.DefaultClient
	}

	models, err := fetchProviderModels(ctx, client, p.BaseURL, p.APIKey)
	if err != nil {
		return InferenceConnection{}, err
	}
	return s.ReplaceInferenceConnectionModels(ctx, id, models, time.Now().UTC())
}

func fetchProviderModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]ModelInfo, error) {
	url := baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxModelsErrorBody))
		if readErr != nil {
			return nil, fmt.Errorf("models HTTP %s: read error body: %w", resp.Status, readErr)
		}
		trimmed := strings.TrimSpace(string(snippet))
		return nil, fmt.Errorf("models HTTP %s: %s", resp.Status, trimmed)
	}

	var list modelsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	models := make([]ModelInfo, 0, len(list.Data))
	for _, m := range list.Data {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		models = append(models, ModelInfo{ID: m.ID, Name: name})
	}
	return models, nil
}

// ModelSupported reports whether the plane has an adapter for the model. A
// model the synced specs cover is supported when its wire mode is known; any
// other OpenCode model falls back to the gemini-/jev- id prefixes (Google wire
// and SystemOne). Other connection types are always supported.
func (s *Store) ModelSupported(connType, baseURL, id string) bool {
	if !IsOpenCodeType(connType) {
		return true
	}
	var prov *modelspecs.Provider
	if s.Specs != nil {
		if p, covered := s.Specs.ProviderFor(connType, baseURL); covered {
			prov = &p
		}
	}
	return openCodeModelSupported(prov, id)
}

// openCodeModelSupported is ModelSupported for an OpenCode model with the
// connection's specs provider already resolved (nil when the specs do not cover it).
func openCodeModelSupported(prov *modelspecs.Provider, id string) bool {
	if prov != nil {
		if spec, found := prov.Lookup(id); found {
			return prov.WireMode(spec) != ""
		}
	}
	lower := strings.ToLower(strings.TrimSpace(id))
	return !strings.HasPrefix(lower, "gemini-") && !strings.HasPrefix(lower, "jev-")
}
