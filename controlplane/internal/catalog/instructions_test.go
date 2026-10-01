package catalog

import "testing"

func TestComposeEffectiveInstructions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		harness   string
		assistant string
		want      string
	}{
		{name: "both empty", want: ""},
		{name: "whitespace only", harness: "  \n", assistant: "\t", want: ""},
		{
			name:    "harness only",
			harness: "Use tools carefully.",
			want:    "<harness_instructions>\nUse tools carefully.\n</harness_instructions>",
		},
		{
			name:      "assistant only",
			assistant: "You are a code reviewer.",
			want:      "<assistant_instructions>\nYou are a code reviewer.\n</assistant_instructions>",
		},
		{
			name:      "harness then assistant",
			harness:   "Global policy.",
			assistant: "Role-specific.",
			want: "<harness_instructions>\nGlobal policy.\n</harness_instructions>\n\n" +
				"<assistant_instructions>\nRole-specific.\n</assistant_instructions>",
		},
		{
			name:      "trims surrounding whitespace",
			harness:   "  outer  ",
			assistant: "\ninner\n",
			want: "<harness_instructions>\nouter\n</harness_instructions>\n\n" +
				"<assistant_instructions>\ninner\n</assistant_instructions>",
		},
		{
			name:    "preserves internal newlines",
			harness: "line1\nline2",
			want:    "<harness_instructions>\nline1\nline2\n</harness_instructions>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ComposeEffectiveInstructions(tc.harness, tc.assistant)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
