package catalog

import "testing"

func TestComposeEffectiveInstructions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		platform       string
		assistant      string
		runtimeContext string
		want           string
	}{
		{name: "all empty", want: ""},
		{name: "whitespace only", platform: "  \n", assistant: "\t", runtimeContext: " ", want: ""},
		{
			name:     "platform only",
			platform: "Use tools carefully.",
			want:     "<platform_instructions>\nUse tools carefully.\n</platform_instructions>",
		},
		{
			name:      "assistant only",
			assistant: "You are a code reviewer.",
			want:      "<assistant_instructions>\nYou are a code reviewer.\n</assistant_instructions>",
		},
		{
			name:           "runtime only",
			runtimeContext: "Date: {{currentDate}}",
			want:           "<runtime_context>\nDate: {{currentDate}}\n</runtime_context>",
		},
		{
			name:           "platform then assistant then runtime",
			platform:       "Global policy.",
			assistant:      "Role-specific.",
			runtimeContext: "Workspace: {{workspaceRoot}}",
			want: "<platform_instructions>\nGlobal policy.\n</platform_instructions>\n\n" +
				"<assistant_instructions>\nRole-specific.\n</assistant_instructions>\n\n" +
				"<runtime_context>\nWorkspace: {{workspaceRoot}}\n</runtime_context>",
		},
		{
			name:      "trims surrounding whitespace",
			platform:  "  outer  ",
			assistant: "\ninner\n",
			want: "<platform_instructions>\nouter\n</platform_instructions>\n\n" +
				"<assistant_instructions>\ninner\n</assistant_instructions>",
		},
		{
			name:     "preserves internal newlines",
			platform: "line1\nline2",
			want:     "<platform_instructions>\nline1\nline2\n</platform_instructions>",
		},
		{
			name:      "preserves unicode, blank lines, and indentation",
			platform:  "\n  Svara på svenska — åäö 日本語 🚀\n\n  - punkt ett\n\t- punkt två\n",
			assistant: "Café ☕\r\nrad två",
			want: "<platform_instructions>\nSvara på svenska — åäö 日本語 🚀\n\n  - punkt ett\n\t- punkt två\n</platform_instructions>\n\n" +
				"<assistant_instructions>\nCafé ☕\r\nrad två\n</assistant_instructions>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ComposeEffectiveInstructions(tc.platform, tc.assistant, tc.runtimeContext)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyInstructionVars(t *testing.T) {
	t.Parallel()

	vars := InstructionVars{
		CurrentDate:   "2026-10-02",
		Timezone:      "Europe/Stockholm",
		WorkspaceRoot: "/workspace",
		ModelID:       "gpt-5",
	}
	in := "Date {{currentDate}} tz {{timezone}} root {{workspaceRoot}} model {{modelId}} keep {{unknown}}"
	want := "Date 2026-10-02 tz Europe/Stockholm root /workspace model gpt-5 keep {{unknown}}"
	if got := ApplyInstructionVars(in, vars); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := ApplyInstructionVars("", vars); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := ApplyInstructionVars("no vars", vars); got != "no vars" {
		t.Fatalf("passthrough = %q", got)
	}
	if got := ApplyInstructionVars("åäö 日本語\n\n  {{modelId}} 🚀 {braces}", vars); got != "åäö 日本語\n\n  gpt-5 🚀 {braces}" {
		t.Fatalf("unicode/newlines = %q", got)
	}
}

func TestApplyInstructionVarsAcrossCompose(t *testing.T) {
	t.Parallel()

	composed := ComposeEffectiveInstructions(
		"Prefer {{modelId}}",
		"Review in {{timezone}}",
		"Today is {{currentDate}} at {{workspaceRoot}}",
	)
	got := ApplyInstructionVars(composed, InstructionVars{
		CurrentDate:   "2026-01-15",
		Timezone:      "UTC",
		WorkspaceRoot: "/workspace",
		ModelID:       "m1",
	})
	want := "<platform_instructions>\nPrefer m1\n</platform_instructions>\n\n" +
		"<assistant_instructions>\nReview in UTC\n</assistant_instructions>\n\n" +
		"<runtime_context>\nToday is 2026-01-15 at /workspace\n</runtime_context>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
