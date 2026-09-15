package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool describes a callable sandbox operation.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Requires    Capabilities
	Run         func(
		ctx context.Context,
		env Environment,
		args json.RawMessage,
	) (string, error)
}

// Definition is the nested OpenAI Chat Completions tool definition shape.
type Definition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
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

func (r *Registry) Definitions(env Environment) []Definition {
	definitions := make([]Definition, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		if !env.Caps().Satisfies(tool.Requires) {
			continue
		}
		definitions = append(definitions, Definition{
			Type: "function",
			Function: FunctionDef{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return definitions
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
	result, marshalErr := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: err.Error()})
	if marshalErr != nil {
		return `{"error":"failed to encode tool error"}`
	}
	return string(result)
}
