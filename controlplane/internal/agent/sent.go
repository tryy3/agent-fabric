package agent

import (
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// sentMessagePart builds the transcript part for the outbound system prompt
// (effective instructions). Chat history is not included — that already
// appears as user/assistant/tool blocks in the conversation.
func sentMessagePart(instructions string) catalog.MessagePart {
	return catalog.MessagePart{
		Type: "sent",
		Text: strings.TrimSpace(instructions),
	}
}

func hasSentPart(parts []catalog.MessagePart) bool {
	for _, p := range parts {
		if p.Type == "sent" {
			return true
		}
	}
	return false
}
