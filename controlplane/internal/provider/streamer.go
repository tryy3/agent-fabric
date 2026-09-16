package provider

import (
	"context"
	"encoding/json"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type ToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Usage struct {
	PromptTokens       *int
	CompletionTokens   *int
	TotalTokens        *int
	PromptMs           *float64
	PredictedMs        *float64
	PromptPerSecond    *float64
	PredictedPerSecond *float64
	Deltas             int
	TTFTMs             *int64
	ElapsedMs          *int64
}

type StreamEvent struct {
	Thought   string
	Content   string
	Finish    string
	Usage     *Usage
	ToolCalls []ToolCall
}

type StreamChatOptions struct {
	Tools []ToolDefinition
}

type ChatStreamer interface {
	StreamChat(ctx context.Context, model string, messages []runtime.Message, opts StreamChatOptions, onEvent func(StreamEvent) error) error
}

func PromptText(blocks []acp.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Text != nil {
			out += b.Text.Text
		}
	}
	return out
}

var _ ChatStreamer = (*OpenAI)(nil)
