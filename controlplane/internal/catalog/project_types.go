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

type Environment struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Spec       json.RawMessage `json:"spec"`
	VolumeName *string         `json:"volumeName"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}
