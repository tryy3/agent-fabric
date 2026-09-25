package provider

import (
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// OpenCode API wire modes (Hermes-compatible names).
const (
	APIModeChatCompletions   = "chat_completions"
	APIModeAnthropicMessages = "anthropic_messages"
	APIModeCodexResponses    = "codex_responses"
)

// Prefix → mode rules checked in order per OpenCode family.
// Sourced from OpenCode's published Zen/Go endpoint tables / Hermes routing.
var openCodeAPIModePrefixes = map[string][]struct {
	prefixes []string
	mode     string
}{
	catalog.TypeOpenCodeGo: {
		{prefixes: []string{"gpt-", "grok-", "muse-spark"}, mode: APIModeCodexResponses},
		{prefixes: []string{"minimax-", "qwen", "union-alpha"}, mode: APIModeAnthropicMessages},
	},
	catalog.TypeOpenCodeZen: {
		{prefixes: []string{"claude-", "union-alpha"}, mode: APIModeAnthropicMessages},
		{prefixes: []string{"gpt-", "grok-", "muse-spark"}, mode: APIModeCodexResponses},
		{prefixes: []string{"qwen"}, mode: APIModeAnthropicMessages},
	},
}

// OpenCodeModelAPIMode returns the wire API for a Zen/Go model id.
// Unknown families default to chat_completions.
func OpenCodeModelAPIMode(providerType, modelID string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if i := strings.LastIndex(normalized, "/"); i >= 0 {
		normalized = normalized[i+1:]
	}
	if normalized == "" {
		return APIModeChatCompletions
	}
	for _, rule := range openCodeAPIModePrefixes[providerType] {
		for _, prefix := range rule.prefixes {
			if strings.HasPrefix(normalized, prefix) {
				return rule.mode
			}
		}
	}
	return APIModeChatCompletions
}
