package catalog

import (
	"encoding/json"
	"time"
)

const (
	TypeOpenAICompatible = "openai_compatible"
	TypeOpenCodeZen      = "opencode_zen"
	TypeOpenCodeGo       = "opencode_go"

	OpenCodeZenBaseURL = "https://opencode.ai/zen/v1"
	OpenCodeGoBaseURL  = "https://opencode.ai/zen/go/v1"
)

// FixedBaseURL returns the canonical base URL for built-in provider types.
// Empty string means the caller must supply a base URL.
func FixedBaseURL(typ string) string {
	switch typ {
	case TypeOpenCodeZen:
		return OpenCodeZenBaseURL
	case TypeOpenCodeGo:
		return OpenCodeGoBaseURL
	default:
		return ""
	}
}

// DefaultProviderName returns a display name for built-in types when create omits one.
func DefaultProviderName(typ string) string {
	switch typ {
	case TypeOpenCodeZen:
		return "OpenCode Zen"
	case TypeOpenCodeGo:
		return "OpenCode Go"
	default:
		return ""
	}
}

// IsOpenCodeType reports whether typ is an OpenCode Zen/Go family provider.
func IsOpenCodeType(typ string) bool {
	return typ == TypeOpenCodeZen || typ == TypeOpenCodeGo
}

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
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Version      int             `json:"version"`
	ProviderID   *string         `json:"providerId"`
	ProviderName *string         `json:"providerName,omitempty"`
	DefaultModel *string         `json:"defaultModel"`
	Settings     json.RawMessage `json:"settings"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

func (a Agent) IsComplete() bool {
	return a.ProviderID != nil && *a.ProviderID != "" &&
		a.DefaultModel != nil && *a.DefaultModel != ""
}
