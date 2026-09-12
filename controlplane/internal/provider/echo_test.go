package provider_test

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestPromptTextConcatenatesTextBlocks(t *testing.T) {
	got := provider.PromptText([]acp.ContentBlock{
		acp.TextBlock("hello"),
		acp.TextBlock(" "),
		acp.TextBlock("world"),
	})
	if got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestEchoReturnsInput(t *testing.T) {
	if got := provider.Echo("ping"); got != "ping" {
		t.Fatalf("got %q", got)
	}
}
