package catalog

import (
	"encoding/json"
	"time"
)

const (
	IsolationIsolated   = "isolated"
	IsolationShared     = "shared"
	PersonalProjectName = "Personal"
)

type Project struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Isolation     string          `json:"isolation"`
	EnvironmentID *string         `json:"environmentId"`
	Settings      json.RawMessage `json:"settings"` // sandbox, allowedAgents, tools, mcp, memory, context
	Remotes       json.RawMessage `json:"remotes"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
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

type Environment struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Spec       json.RawMessage `json:"spec"`
	VolumeName *string         `json:"volumeName"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}
