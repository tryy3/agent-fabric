package catalog

import "time"

type Checkpoint struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	SHA       string    `json:"sha"`
	Label     string    `json:"label"`
	ThreadID  *string   `json:"threadId,omitempty"`
	MessageID *string   `json:"messageId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type GitCommit struct {
	SHA          string    `json:"sha"`
	Message      string    `json:"message"`
	CommittedAt  time.Time `json:"committedAt"`
	CheckpointID *string   `json:"checkpointId,omitempty"`
	Label        *string   `json:"label,omitempty"`
}

type CommitList struct {
	Commits []GitCommit `json:"commits"`
}

type DiffResult struct {
	From string `json:"from"`
	To   string `json:"to"`
	Diff string `json:"diff"`
}
