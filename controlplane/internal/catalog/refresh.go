package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxModelsErrorBody = 4 << 10

type modelsListResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

func (s *Store) RefreshModels(ctx context.Context, id string, client *http.Client) (Provider, error) {
	p, ok := s.GetProvider(id)
	if !ok {
		return Provider{}, fmt.Errorf("provider %q not found", id)
	}
	if !isKnownProviderType(p.Type) {
		return Provider{}, fmt.Errorf("unknown provider type %q", p.Type)
	}
	if client == nil {
		client = http.DefaultClient
	}

	models, err := fetchProviderModels(ctx, client, p.BaseURL, p.APIKey)
	if err != nil {
		return Provider{}, err
	}
	return s.ReplaceProviderModels(id, models, time.Now().UTC())
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
