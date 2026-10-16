package catalog

import "time"

// Publish is one successful publish of a project's files to a publish
// provider. SiteURL is the provider's always-latest link at publish time;
// DeployURL stays pinned to this publish.
type Publish struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Provider  string `json:"provider"`
	Target    string `json:"target,omitempty"`
	DeployID  string `json:"deployId,omitempty"`
	SiteURL   string `json:"siteUrl,omitempty"`
	DeployURL string `json:"deployUrl,omitempty"`
	// Live marks the deploy the target currently serves at its latest link.
	Live      bool      `json:"live"`
	CreatedAt time.Time `json:"createdAt"`
}

// PublishInput is what a publish provider reports about one deploy: after a
// successful publish, or for each deploy it lists during publish sync.
type PublishInput struct {
	Provider  string
	Target    string
	DeployID  string
	SiteURL   string
	DeployURL string
	// Live is only read by ReconcilePublishes; InsertPublish always makes the
	// new publish live.
	Live bool
	// CreatedAt is when the provider created the deploy; zero means now.
	CreatedAt time.Time
}

// PublishSyncStats counts what ReconcilePublishes changed for one target.
type PublishSyncStats struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Updated int `json:"updated"`
}
