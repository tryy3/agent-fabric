package catalog

import "time"

type TitleSource string

const (
	TitleSourceAuto TitleSource = "auto"
	TitleSourceUser TitleSource = "user"
)

type Thread struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	TitleSource  TitleSource `json:"titleSource"`
	AgentID      *string     `json:"agentId"`
	CurrentModel *string     `json:"currentModel"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

type ThreadListItem struct {
	Thread
	MessageCount int `json:"messageCount"`
}

type ThreadMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type ThreadDetail struct {
	Thread
	Messages []ThreadMessage `json:"messages"`
}
