// Package gateinfo implements the plane-owned read_gate_instructions tool: a
// read-only view of the gate instructions pinned for the session. The agent
// has no way to change them.
package gateinfo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

const Name = "read_gate_instructions"

// NoneText is the result when the session has no gate instructions.
const NoneText = "No gate instructions are set for this session."

// Tools returns the read_gate_instructions sandbox tool (no FS/Exec requirements).
func Tools() []sandbox.Tool {
	return []sandbox.Tool{{
		Name: Name,
		Description: "Read the user's gate instructions: their rules about what you may and may not do. " +
			"The gate checks tool calls against these rules. You cannot change them.",
		Parameters: sandbox.Parameters{Properties: map[string]sandbox.Property{}},
		Requires:   sandbox.Capabilities{},
		Run: func(context.Context, sandbox.Environment, json.RawMessage) (string, error) {
			return "", fmt.Errorf("%s must be handled by the agent, which holds the session pin", Name)
		},
	}}
}

// Result is what the tool returns for the session's pinned gate instructions.
func Result(instructions string) string {
	if instructions == "" {
		return NoneText
	}
	return instructions
}
