package modelspecs

// Wire API modes; the values match the provider package's APIMode constants.
const (
	WireChatCompletions   = "chat_completions"
	WireAnthropicMessages = "anthropic_messages"
	WireCodexResponses    = "codex_responses"
)

// WireModeFromNPM maps a models.dev SDK package to the wire API it speaks.
// It returns "" for packages we have no adapter for, so callers fall back to
// their own routing instead of guessing.
func WireModeFromNPM(npm string) string {
	switch npm {
	case "@ai-sdk/anthropic":
		return WireAnthropicMessages
	case "@ai-sdk/openai":
		return WireCodexResponses
	case "@ai-sdk/openai-compatible":
		return WireChatCompletions
	}
	return ""
}

// WireMode is the wire API for a model: its own provider override when it has
// one, else the provider's package. "" means unknown.
func (p Provider) WireMode(m Model) string {
	if m.Provider != nil && m.Provider.NPM != "" {
		return WireModeFromNPM(m.Provider.NPM)
	}
	return WireModeFromNPM(p.NPM)
}
