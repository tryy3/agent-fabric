package catalog

import (
	"encoding/json"
	"time"
)

const (
	DefaultProjectName = "Default"
)

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Settings    json.RawMessage `json:"settings"` // sandbox, allowedAgents, tools, mcp, memory, context
	Remotes     json.RawMessage `json:"remotes"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Remote is a project remotes[] stub. Tokens stay on Providers, never here.
type Remote struct {
	ID          string `json:"id"`
	ProviderID  string `json:"providerId,omitempty"`
	Kind        string `json:"kind"`
	URLOrBucket string `json:"urlOrBucket,omitempty"`
	Path        string `json:"path,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}
