package catalog

import (
	"time"

	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

type TitleSource string

const (
	TitleSourceAuto TitleSource = "auto"
	TitleSourceUser TitleSource = "user"
)

type Thread struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	TitleSource  TitleSource `json:"titleSource"`
	AssistantID  *string     `json:"assistantId"`
	CurrentModel *string     `json:"currentModel"`
	ViewModeID   *string     `json:"viewModeId"`
	ProjectID    string      `json:"projectId"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

type ThreadListItem struct {
	Thread
	MessageCount int `json:"messageCount"`
}

type MessagePart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Name       string `json:"name,omitempty"`
	Title      string `json:"title,omitempty"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
	Status     string `json:"status,omitempty"`
	// Gate is what the tool gate decided for a tool_call part (risk score,
	// band, outcome, per-evaluator scores). Never sent to the model.
	Gate               map[string]any `json:"gate,omitempty"`
	PromptTokens       *int           `json:"promptTokens,omitempty"`
	CompletionTokens   *int           `json:"completionTokens,omitempty"`
	TotalTokens        *int           `json:"totalTokens,omitempty"`
	ContextUsed        *int           `json:"contextUsed,omitempty"`
	ContextSize        *int           `json:"contextSize,omitempty"`
	PromptMs           *float64       `json:"promptMs,omitempty"`
	PredictedMs        *float64       `json:"predictedMs,omitempty"`
	TTFTMs             *int64         `json:"ttftMs,omitempty"`
	ElapsedMs          *int64         `json:"elapsedMs,omitempty"`
	PromptPerSecond    *float64       `json:"promptPerSecond,omitempty"`
	PredictedPerSecond *float64       `json:"predictedPerSecond,omitempty"`
	Co2Grams           *float64       `json:"co2Grams,omitempty"`
	GpuEnergyJoules    *float64       `json:"gpuEnergyJoules,omitempty"`
	Deltas             *int           `json:"deltas,omitempty"`

	CachedTokens     *int `json:"cachedTokens,omitempty"`
	CacheWriteTokens *int `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  *int `json:"reasoningTokens,omitempty"`
}

// MessageCost is an estimated cost in USD (prices per million tokens come
// from synced model specs, so it is never exact billing).
type MessageCost struct {
	Currency  string `json:"currency"`
	Estimated bool   `json:"estimated"`
	modelspecs.Breakdown
}

// MessageRound is what the plane computed for one LLM call of an assistant
// attempt. It lives beside the message (table message_rounds), never in its
// parts, which stay what was exchanged with the provider.
type MessageRound struct {
	Round int    `json:"round"`
	Model string `json:"model,omitempty"`
	// PartIndex is the index into the message's parts of the first part this
	// round produced, so a client can show the round where it begins.
	PartIndex int `json:"partIndex"`
	// The counts the cost was calculated from.
	PromptTokens     *int `json:"promptTokens,omitempty"`
	CompletionTokens *int `json:"completionTokens,omitempty"`
	CachedTokens     *int `json:"cachedTokens,omitempty"`
	CacheWriteTokens *int `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  *int `json:"reasoningTokens,omitempty"`
	// Cost is the plane's estimate; nil when the model has no published price.
	// ReportedCostUSD is what the provider itself reported, when it did.
	Cost            *MessageCost `json:"cost,omitempty"`
	ReportedCostUSD *float64     `json:"reportedCostUsd,omitempty"`
}

// ThreadTotals sums every LLM call of every assistant attempt in a thread,
// including retried and failed ones: what the thread actually cost.
type ThreadTotals struct {
	Turns            int `json:"turns"`
	Requests         int `json:"requests"`
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	CachedTokens     int `json:"cachedTokens"`
	CacheWriteTokens int `json:"cacheWriteTokens"`
	ReasoningTokens  int `json:"reasoningTokens"`
	// Cost is nil when no request had a price. Partial is set when only some
	// did, or a price component was unknown.
	Cost            *MessageCost `json:"cost,omitempty"`
	ReportedCostUSD *float64     `json:"reportedCostUsd,omitempty"`
}

// AttemptStatus is the lifecycle of an assistant attempt (messages.status).
type AttemptStatus string

const (
	AttemptStatusRunning   AttemptStatus = "running"
	AttemptStatusCompleted AttemptStatus = "completed"
	AttemptStatusFailed    AttemptStatus = "failed"
	AttemptStatusCancelled AttemptStatus = "cancelled"
)

type AssistantTurn struct {
	Content      string
	Model        string
	ProviderID   string
	ProviderName string
	StopReason   string
	Parts        []MessagePart
	// Rounds is the plane-computed cost of each LLM call, stored beside the message.
	Rounds []MessageRound
	// CaptureSessionID, when set, links in-flight hop_captures for that ACP session to the assistant message.
	CaptureSessionID string
}

// TurnHandles identifies the durable rows opened by BeginTurn / BeginAssistantAttempt.
type TurnHandles struct {
	UserMessageID      string
	AssistantMessageID string
}

type ThreadMessage struct {
	ID           string         `json:"id"`
	Role         string         `json:"role"`
	Content      string         `json:"content"`
	Position     int            `json:"position"`
	CreatedAt    time.Time      `json:"createdAt"`
	Model        *string        `json:"model,omitempty"`
	ProviderID   *string        `json:"providerId,omitempty"`
	ProviderName *string        `json:"providerName,omitempty"`
	StopReason   *string        `json:"stopReason,omitempty"`
	Parts        []MessagePart  `json:"parts"`
	Rounds       []MessageRound `json:"rounds,omitempty"`
	// Cost and ReportedCostUSD sum this message's rounds.
	Cost            *MessageCost `json:"cost,omitempty"`
	ReportedCostUSD *float64     `json:"reportedCostUsd,omitempty"`
	Active          bool         `json:"active"`
	PromptMessageID *string      `json:"promptMessageId,omitempty"`
	Status          string       `json:"status"`
}

// RetryTarget is the latest completed user+assistant pair eligible for soft-supersede retry.
type RetryTarget struct {
	UserMessageID      string
	UserText           string
	AssistantMessageID string
}

type ThreadDetail struct {
	Thread
	MessageCount int             `json:"messageCount"`
	Messages     []ThreadMessage `json:"messages"`
	Totals       ThreadTotals    `json:"totals"`
}
