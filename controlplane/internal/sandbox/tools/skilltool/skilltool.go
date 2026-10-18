// Package skilltool implements the plane-owned load_skill tool: a read-only
// view of the skills pinned for the session. The agent reads a skill's
// SKILL.md or one of its extra files; it cannot change either.
package skilltool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

const Name = "load_skill"

// Tools returns the load_skill sandbox tool (no FS/Exec requirements).
func Tools() []sandbox.Tool {
	return []sandbox.Tool{{
		Name: Name,
		Description: "Load a skill by name from the available skills list. " +
			"With only a name it returns the skill's SKILL.md instructions; " +
			"with a path it returns that extra file from the skill's folder (for example a file under references/). " +
			"Skills are read-only instructions. Follow them with your normal tools.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"name": {Type: "string", Description: "Skill name from the available skills list."},
				"path": {Type: "string", Description: "Optional path of an extra file inside the skill folder, relative to it."},
			},
			Required: []string{"name"},
		},
		Requires: sandbox.Capabilities{},
		Run: func(context.Context, sandbox.Environment, json.RawMessage) (string, error) {
			return "", fmt.Errorf("%s must be handled by the agent, which holds the session pin", Name)
		},
	}}
}

// Args are the decoded load_skill arguments.
type Args struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ParseArgs decodes and trims the arguments of a load_skill call.
func ParseArgs(raw json.RawMessage) (Args, error) {
	var a Args
	if err := json.Unmarshal(raw, &a); err != nil {
		return Args{}, fmt.Errorf("invalid load_skill arguments: %w", err)
	}
	a.Name = strings.TrimSpace(a.Name)
	a.Path = strings.TrimSpace(a.Path)
	if a.Name == "" {
		return Args{}, fmt.Errorf("name is required")
	}
	return a, nil
}
