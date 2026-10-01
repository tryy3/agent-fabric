package catalog

import "strings"

// ComposeEffectiveInstructions builds provider-independent effective instructions
// from Harness then Assistant sources. Empty segments are omitted; present
// segments keep named boundaries so sources stay distinguishable.
func ComposeEffectiveInstructions(harness, assistant string) string {
	harness = strings.TrimSpace(harness)
	assistant = strings.TrimSpace(assistant)
	var parts []string
	if harness != "" {
		parts = append(parts, "<harness_instructions>\n"+harness+"\n</harness_instructions>")
	}
	if assistant != "" {
		parts = append(parts, "<assistant_instructions>\n"+assistant+"\n</assistant_instructions>")
	}
	return strings.Join(parts, "\n\n")
}
