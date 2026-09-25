package provider_test

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestOpenCodeModelAPIMode(t *testing.T) {
	cases := []struct {
		typ, model, want string
	}{
		{catalog.TypeOpenCodeGo, "deepseek-v4-flash", provider.APIModeChatCompletions},
		{catalog.TypeOpenCodeGo, "kimi-k3", provider.APIModeChatCompletions},
		{catalog.TypeOpenCodeGo, "glm-5.3", provider.APIModeChatCompletions},
		{catalog.TypeOpenCodeGo, "gpt-6-luna", provider.APIModeCodexResponses},
		{catalog.TypeOpenCodeGo, "grok-4.7", provider.APIModeCodexResponses},
		{catalog.TypeOpenCodeGo, "muse-spark-1.3-contributor", provider.APIModeCodexResponses},
		{catalog.TypeOpenCodeGo, "minimax-m3", provider.APIModeAnthropicMessages},
		{catalog.TypeOpenCodeGo, "qwen3.8-flash", provider.APIModeAnthropicMessages},
		{catalog.TypeOpenCodeZen, "claude-opus-4-6", provider.APIModeAnthropicMessages},
		{catalog.TypeOpenCodeZen, "claude-sonnet-5", provider.APIModeAnthropicMessages},
		{catalog.TypeOpenCodeZen, "gpt-5.5", provider.APIModeCodexResponses},
		{catalog.TypeOpenCodeZen, "grok-4.6", provider.APIModeCodexResponses},
		{catalog.TypeOpenCodeZen, "qwen3.7-plus", provider.APIModeAnthropicMessages},
		{catalog.TypeOpenCodeZen, "deepseek-v4-pro", provider.APIModeChatCompletions},
		{catalog.TypeOpenCodeZen, "kimi-k2.6", provider.APIModeChatCompletions},
		{catalog.TypeOpenCodeZen, "opencode/gpt-5.5", provider.APIModeCodexResponses},
		{catalog.TypeOpenAICompatible, "anything", provider.APIModeChatCompletions},
	}
	for _, tc := range cases {
		got := provider.OpenCodeModelAPIMode(tc.typ, tc.model)
		if got != tc.want {
			t.Errorf("OpenCodeModelAPIMode(%q, %q) = %q, want %q", tc.typ, tc.model, got, tc.want)
		}
	}
}
