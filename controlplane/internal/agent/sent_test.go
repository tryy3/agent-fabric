package agent

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestSentMessagePart(t *testing.T) {
	t.Parallel()
	part := sentMessagePart("  <platform_instructions>\nABC\n</platform_instructions>  ")
	if part.Type != "sent" {
		t.Fatalf("type = %q", part.Type)
	}
	if part.Text != "<platform_instructions>\nABC\n</platform_instructions>" {
		t.Fatalf("text = %q", part.Text)
	}
	empty := sentMessagePart(" \n\t ")
	if empty != (catalog.MessagePart{Type: "sent"}) {
		t.Fatalf("empty = %+v", empty)
	}
}
