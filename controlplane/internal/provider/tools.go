package provider

import "encoding/json"

// FunctionTool builds an OpenAI Chat Completions function-tool definition.
// parameters should be a JSON Schema object (or anything that marshals to one).
func FunctionTool(name, description string, parameters any) (ToolDefinition, error) {
	raw, err := json.Marshal(parameters)
	if err != nil {
		return ToolDefinition{}, err
	}
	def := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        name,
			Description: description,
			Parameters:  raw,
		},
	}
	return def, nil
}
