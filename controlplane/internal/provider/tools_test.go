package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestFunctionToolWrapsJSONSchemaAsOpenAIFunction(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}

	def, err := provider.FunctionTool("read_file", "Read a file", params)
	if err != nil {
		t.Fatal(err)
	}
	if def.Type != "function" {
		t.Fatalf("Type = %q", def.Type)
	}
	if def.Function.Name != "read_file" || def.Function.Description != "Read a file" {
		t.Fatalf("function = %+v", def.Function)
	}
	var schema map[string]any
	if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Fatalf("parameters = %s", def.Function.Parameters)
	}
}
