package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
)

// Property describes one JSON Schema property for tool arguments.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Parameters is a JSON Schema object describing tool arguments.
// It is provider-neutral; OpenAI (or others) adapt it at the call site.
type Parameters struct {
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// MarshalJSON emits a JSON Schema object with type "object".
func (p Parameters) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type                 string              `json:"type"`
		Properties           map[string]Property `json:"properties,omitempty"`
		Required             []string            `json:"required,omitempty"`
		AdditionalProperties bool                `json:"additionalProperties"`
	}
	return json.Marshal(wire{
		Type:                 "object",
		Properties:           p.Properties,
		Required:             p.Required,
		AdditionalProperties: false,
	})
}

// Tool describes a callable sandbox operation.
type Tool struct {
	Name        string
	Description string
	Parameters  Parameters
	Requires    Capabilities
	Run         func(
		ctx context.Context,
		env Environment,
		args json.RawMessage,
	) (string, error)
}

type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) {
	if _, exists := r.tools[tool.Name]; !exists {
		r.order = append(r.order, tool.Name)
	}
	r.tools[tool.Name] = tool
}

// Available returns tools whose requirements are satisfied by env.
func (r *Registry) Available(env Environment) []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		if !env.Caps().Satisfies(tool.Requires) {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func (r *Registry) Call(
	ctx context.Context,
	env Environment,
	name string,
	args json.RawMessage,
) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if !env.Caps().Satisfies(tool.Requires) {
		return errorResult(fmt.Errorf("tool %q requirements are not satisfied", name)), nil
	}
	if tool.Run == nil {
		return errorResult(fmt.Errorf("tool %q has no runner", name)), nil
	}
	out, err := tool.Run(ctx, env, args)
	if err != nil {
		return errorResult(err), nil
	}
	return out, nil
}

func errorResult(err error) string {
	b, mErr := json.Marshal(map[string]string{"error": err.Error()})
	if mErr != nil {
		return `{"error":"unknown error"}`
	}
	return string(b)
}
