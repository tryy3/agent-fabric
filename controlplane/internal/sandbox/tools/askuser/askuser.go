// Package askuser implements the plane-owned ask_user clarification tool.
package askuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

const Name = "ask_user"

// Option is one multiple-choice option for a question.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Question is one clarification question (1–3 per call).
type Question struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
	Multi    bool     `json:"multi,omitempty"`
}

// Args is the model-facing ask_user payload.
type Args struct {
	Questions []Question `json:"questions"`
}

// Tools returns the ask_user sandbox tool (no FS/Exec requirements).
func Tools() []sandbox.Tool {
	return []sandbox.Tool{{
		Name: Name,
		Description: "Ask the user 1–3 short multiple-choice questions when the task is ambiguous. " +
			"Do not include an Other option; the client adds one. Prefer sensible defaults — only ask when the answer changes what you do next.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"questions": {
					Type:        "array",
					Description: "One to three questions. Each needs id, question text, and 2–4 options with label (and optional description).",
				},
			},
			Required: []string{"questions"},
		},
		Requires: sandbox.Capabilities{},
		Run: func(context.Context, sandbox.Environment, json.RawMessage) (string, error) {
			return "", fmt.Errorf("ask_user must be handled by the agent elicitation path")
		},
	}}
}

// ParseAndValidate decodes and validates ask_user arguments.
func ParseAndValidate(raw json.RawMessage) (Args, error) {
	var args Args
	if err := json.Unmarshal(raw, &args); err != nil {
		return Args{}, fmt.Errorf("decode ask_user arguments: %w", err)
	}
	n := len(args.Questions)
	if n < 1 || n > 3 {
		return Args{}, fmt.Errorf("ask_user requires 1–3 questions, got %d", n)
	}
	seen := make(map[string]struct{}, n)
	for i := range args.Questions {
		q := &args.Questions[i]
		q.ID = strings.TrimSpace(q.ID)
		q.Question = strings.TrimSpace(q.Question)
		if q.ID == "" {
			return Args{}, fmt.Errorf("questions[%d].id is required", i)
		}
		if _, ok := seen[q.ID]; ok {
			return Args{}, fmt.Errorf("duplicate question id %q", q.ID)
		}
		seen[q.ID] = struct{}{}
		if q.Question == "" {
			return Args{}, fmt.Errorf("questions[%d].question is required", i)
		}
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return Args{}, fmt.Errorf("questions[%d] needs 2–4 options, got %d", i, len(q.Options))
		}
		for j := range q.Options {
			q.Options[j].Label = strings.TrimSpace(q.Options[j].Label)
			q.Options[j].Description = strings.TrimSpace(q.Options[j].Description)
			if q.Options[j].Label == "" {
				return Args{}, fmt.Errorf("questions[%d].options[%d].label is required", i, j)
			}
		}
	}
	return args, nil
}
