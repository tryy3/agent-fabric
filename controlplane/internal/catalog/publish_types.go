package catalog

import "time"

// Publish is one successful publish of a project's files to a publish
// provider. SiteURL is the provider's always-latest link at publish time;
// DeployURL stays pinned to this publish.
type Publish struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Provider  string    `json:"provider"`
	Target    string    `json:"target,omitempty"`
	DeployID  string    `json:"deployId,omitempty"`
	SiteURL   string    `json:"siteUrl,omitempty"`
	DeployURL string    `json:"deployUrl,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// PublishInput is what a publish provider reports after a successful publish.
type PublishInput struct {
	Provider  string
	Target    string
	DeployID  string
	SiteURL   string
	DeployURL string
}
