package catalog

import (
	"encoding/json"
	"time"
)

const (
	TypeOpenAICompatible = "openai_compatible"
	TypeOpenCodeZen      = "opencode_zen"
	TypeOpenCodeGo       = "opencode_go"
	TypeUnslothStudio    = "unsloth_studio"

	OpenCodeZenBaseURL = "https://opencode.ai/zen/v1"
	OpenCodeGoBaseURL  = "https://opencode.ai/zen/go/v1"
)

// FixedBaseURL returns the canonical base URL for built-in connection types.
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

// DefaultInferenceConnectionName returns a display name for built-in types when create omits one.
func DefaultInferenceConnectionName(typ string) string {
	switch typ {
	case TypeOpenCodeZen:
		return "OpenCode Zen"
	case TypeOpenCodeGo:
		return "OpenCode Go"
	case TypeUnslothStudio:
		return "Unsloth Studio"
	default:
		return ""
	}
}

// IsOpenCodeType reports whether typ is an OpenCode Zen/Go family connection type.
func IsOpenCodeType(typ string) bool {
	return typ == TypeOpenCodeZen || typ == TypeOpenCodeGo
}

// ModelInfo is a model reference in an inference connection's model catalog.
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// InferenceConnection is a saved Catalog inference connection.
type InferenceConnection struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Type            string      `json:"type"` // connection type
	BaseURL         string      `json:"baseUrl"`
	APIKey          string      `json:"apiKey"`
	Models          []ModelInfo `json:"models"`
	ModelsUpdatedAt *time.Time  `json:"modelsUpdatedAt,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

// Assistant is a configurable Catalog assistant.
type Assistant struct {
	ID                        string          `json:"id"`
	Name                      string          `json:"name"`
	Description               string          `json:"description,omitempty"`
	Version                   int             `json:"version"`
	InferenceConnectionID     *string         `json:"inferenceConnectionId"`
	InferenceConnectionName   *string         `json:"inferenceConnectionName,omitempty"`
	DefaultModel              *string         `json:"defaultModel"`
	Settings                  json.RawMessage `json:"settings"`
	CreatedAt                 time.Time       `json:"createdAt"`
	UpdatedAt                 time.Time       `json:"updatedAt"`
}

func (a Assistant) IsComplete() bool {
	return a.InferenceConnectionID != nil && *a.InferenceConnectionID != "" &&
		a.DefaultModel != nil && *a.DefaultModel != ""
}
