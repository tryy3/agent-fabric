package provider

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type ChatStreamer interface {
	StreamChat(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error
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
