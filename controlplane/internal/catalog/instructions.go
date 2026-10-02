package catalog

import (
	"strings"
	"time"
)

// InstructionVars holds values substituted into instruction templates at
// session/new (when effective instructions are pinned).
type InstructionVars struct {
	CurrentDate   string
	Timezone      string
	WorkspaceRoot string
	ModelID       string
}

// NewInstructionVars builds vars for the current wall clock in the host local
// timezone, plus the given workspace root and model id.
func NewInstructionVars(workspaceRoot, modelID string) InstructionVars {
	now := time.Now()
	tz := now.Location().String()
	if tz == "" || tz == "Local" {
		if name, _ := now.Zone(); name != "" {
			tz = name
		} else {
			tz = "UTC"
		}
	}
	return InstructionVars{
		CurrentDate:   now.Format("2006-01-02"),
		Timezone:      tz,
		WorkspaceRoot: workspaceRoot,
		ModelID:       modelID,
	}
}

// ApplyInstructionVars replaces known {{placeholders}} in text. Unknown
// placeholders are left unchanged.
func ApplyInstructionVars(text string, vars InstructionVars) string {
	if text == "" || !strings.Contains(text, "{{") {
		return text
	}
	replacer := strings.NewReplacer(
		"{{currentDate}}", vars.CurrentDate,
		"{{timezone}}", vars.Timezone,
		"{{workspaceRoot}}", vars.WorkspaceRoot,
		"{{modelId}}", vars.ModelID,
	)
	return replacer.Replace(text)
}

// ComposeEffectiveInstructions builds provider-independent effective
// instructions from Platform, Assistant, then Runtime Context sources. Empty
// segments are omitted; present segments keep named boundaries so sources stay
// distinguishable. Callers should ApplyInstructionVars before or after compose.
func ComposeEffectiveInstructions(platform, assistant, runtimeContext string) string {
	platform = strings.TrimSpace(platform)
	assistant = strings.TrimSpace(assistant)
	runtimeContext = strings.TrimSpace(runtimeContext)
	var parts []string
	if platform != "" {
		parts = append(parts, "<platform_instructions>\n"+platform+"\n</platform_instructions>")
	}
	if assistant != "" {
		parts = append(parts, "<assistant_instructions>\n"+assistant+"\n</assistant_instructions>")
	}
	if runtimeContext != "" {
		parts = append(parts, "<runtime_context>\n"+runtimeContext+"\n</runtime_context>")
	}
	return strings.Join(parts, "\n\n")
}
