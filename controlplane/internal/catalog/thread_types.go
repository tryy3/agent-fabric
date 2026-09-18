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

type MessagePart struct {
	Type               string   `json:"type"`
	Text               string   `json:"text,omitempty"`
	ToolCallID         string   `json:"toolCallId,omitempty"`
	Name               string   `json:"name,omitempty"`
	Title              string   `json:"title,omitempty"`
	Input              string   `json:"input,omitempty"`
	Output             string   `json:"output,omitempty"`
	Status             string   `json:"status,omitempty"`
	PromptTokens       *int     `json:"promptTokens,omitempty"`
	CompletionTokens   *int     `json:"completionTokens,omitempty"`
	TotalTokens        *int     `json:"totalTokens,omitempty"`
	ContextUsed        *int     `json:"contextUsed,omitempty"`
	ContextSize        *int     `json:"contextSize,omitempty"`
	PromptMs           *float64 `json:"promptMs,omitempty"`
	PredictedMs        *float64 `json:"predictedMs,omitempty"`
	TTFTMs             *int64   `json:"ttftMs,omitempty"`
	ElapsedMs          *int64   `json:"elapsedMs,omitempty"`
	PromptPerSecond    *float64 `json:"promptPerSecond,omitempty"`
	PredictedPerSecond *float64 `json:"predictedPerSecond,omitempty"`
	Deltas             *int     `json:"deltas,omitempty"`
}

type AssistantTurn struct {
	Content      string
	Model        string
	ProviderID   string
	ProviderName string
	StopReason   string
	Parts        []MessagePart
}

type ThreadMessage struct {
	ID           string        `json:"id"`
	Role         string        `json:"role"`
	Content      string        `json:"content"`
	Position     int           `json:"position"`
	CreatedAt    time.Time     `json:"createdAt"`
	Model        *string       `json:"model,omitempty"`
	ProviderID   *string       `json:"providerId,omitempty"`
	ProviderName *string       `json:"providerName,omitempty"`
	StopReason   *string       `json:"stopReason,omitempty"`
	Parts        []MessagePart `json:"parts"`
}

type ThreadDetail struct {
	Thread
	MessageCount int             `json:"messageCount"`
	Messages     []ThreadMessage `json:"messages"`
}
