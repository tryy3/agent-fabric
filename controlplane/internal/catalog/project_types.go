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
	Settings      json.RawMessage `json:"settings"`
	Remotes       json.RawMessage `json:"remotes"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}
