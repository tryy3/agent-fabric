// Package modelspecs syncs a models.dev-shaped specs database (providers,
// models, capabilities, limits, prices) into the plane.
package modelspecs

import "time"

// DefaultSourceURL is the upstream specs source used when none is configured.
const DefaultSourceURL = "https://models.dev/api.json"

const (
	DefaultSyncIntervalHours = 24
	// MaxBodyBytes caps a specs download; api.json is a few MiB today.
	MaxBodyBytes = 32 << 20
)

// Provider is one provider entry of a models.dev api.json document.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	NPM    string           `json:"npm,omitempty"`
	API    string           `json:"api,omitempty"`
	Doc    string           `json:"doc,omitempty"`
	Env    []string         `json:"env,omitempty"`
	Models map[string]Model `json:"models"`
}

// Model is one model entry. Costs are USD per 1M tokens.
type Model struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Family           string      `json:"family,omitempty"`
	Attachment       bool        `json:"attachment"`
	Reasoning        bool        `json:"reasoning"`
	ToolCall         bool        `json:"tool_call"`
	StructuredOutput bool        `json:"structured_output"`
	Temperature      bool        `json:"temperature"`
	OpenWeights      bool        `json:"open_weights"`
	ReleaseDate      string      `json:"release_date,omitempty"`
	LastUpdated      string      `json:"last_updated,omitempty"`
	Knowledge        string      `json:"knowledge,omitempty"`
	Status           string      `json:"status,omitempty"`
	Modalities       *Modalities `json:"modalities,omitempty"`
	Limit            *Limit      `json:"limit,omitempty"`
	Cost             *Cost       `json:"cost,omitempty"`
}

type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type Limit struct {
	Context int `json:"context"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output"`
}

// Cost fields are pointers so an absent price is distinguishable from free.
type Cost struct {
	Input       *float64 `json:"input,omitempty"`
	Output      *float64 `json:"output,omitempty"`
	Reasoning   *float64 `json:"reasoning,omitempty"`
	CacheRead   *float64 `json:"cache_read,omitempty"`
	CacheWrite  *float64 `json:"cache_write,omitempty"`
	InputAudio  *float64 `json:"input_audio,omitempty"`
	OutputAudio *float64 `json:"output_audio,omitempty"`
}

// Settings configures the specs source and sync schedule.
type Settings struct {
	// SourceURL is empty for the default models.dev source.
	SourceURL         string `json:"sourceUrl"`
	SyncIntervalHours int    `json:"syncIntervalHours"`
	Enabled           bool   `json:"enabled"`
}

// EffectiveSourceURL resolves the empty default.
func (s Settings) EffectiveSourceURL() string {
	if s.SourceURL == "" {
		return DefaultSourceURL
	}
	return s.SourceURL
}

// Status describes the settings, the stored snapshot and the last attempt.
type Status struct {
	Settings
	EffectiveSourceURL string     `json:"effectiveSourceUrl"`
	SnapshotSourceURL  string     `json:"snapshotSourceUrl"`
	ProviderCount      int        `json:"providerCount"`
	ModelCount         int        `json:"modelCount"`
	Bytes              int64      `json:"bytes"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt"`
	LastAttemptAt      *time.Time `json:"lastAttemptAt"`
	LastError          string     `json:"lastError"`
}
