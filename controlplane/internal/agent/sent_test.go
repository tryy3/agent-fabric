package agent

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestSentMessagePart(t *testing.T) {
	t.Parallel()
	part := sentMessagePart("  <harness_instructions>\nABC\n</harness_instructions>  ")
	if part.Type != "sent" {
		t.Fatalf("type = %q", part.Type)
	}
	if part.Text != "<harness_instructions>\nABC\n</harness_instructions>" {
		t.Fatalf("text = %q", part.Text)
	}
	empty := sentMessagePart(" \n\t ")
	if empty != (catalog.MessagePart{Type: "sent"}) {
		t.Fatalf("empty = %+v", empty)
	}
}
