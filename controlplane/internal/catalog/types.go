package catalog

import "time"

const TypeOpenAICompatible = "openai_compatible"

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Provider struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Type            string      `json:"type"`
	BaseURL         string      `json:"baseUrl"`
	APIKey          string      `json:"apiKey"`
	Models          []ModelInfo `json:"models"`
	ModelsUpdatedAt *time.Time  `json:"modelsUpdatedAt,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Version      int       `json:"version"`
	ProviderID   string    `json:"providerId"`
	DefaultModel string    `json:"defaultModel"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
