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
	Settings    json.RawMessage `json:"settings"` // sandbox, allowedAssistants, tools, mcp, memory, context
	Remotes     json.RawMessage `json:"remotes"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Remote is a project remotes[] stub. Tokens stay on inference connections, never here.
type Remote struct {
	ID                    string `json:"id"`
	InferenceConnectionID string `json:"inferenceConnectionId,omitempty"`
	Kind                  string `json:"kind"`
	URLOrBucket           string `json:"urlOrBucket,omitempty"`
	Path                  string `json:"path,omitempty"`
	Enabled               *bool  `json:"enabled,omitempty"`
}
