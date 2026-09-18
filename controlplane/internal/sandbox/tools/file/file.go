package file

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type readArgs struct {
	Path string `json:"path"`
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func Tools() []sandbox.Tool {
	requiresFS := sandbox.Capabilities{FS: true}
	return []sandbox.Tool{
		{
			Name:        "read_file",
			Description: "Read the contents of a file in the workspace.",
			Parameters: sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"path": {
						Type:        "string",
						Description: "Path under the workspace root (absolute within root, or relative)",
					},
				},
				Required: []string{"path"},
			},
			Requires: requiresFS,
			Run:      readFile,
		},
		{
			Name:        "write_file",
			Description: "Write content to a file in the workspace, creating parent directories.",
			Parameters: sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"path": {
						Type:        "string",
						Description: "Path under the workspace root (absolute within root, or relative)",
					},
					"content": {
						Type:        "string",
						Description: "File content to write",
					},
				},
				Required: []string{"path", "content"},
			},
			Requires: requiresFS,
			Run:      writeFile,
		},
	}
}

func readFile(
	ctx context.Context,
	env sandbox.Environment,
	raw json.RawMessage,
) (string, error) {
	var args readArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("decode read_file arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("read_file path is required")
	}
	fsys, ok := env.FS()
	if !ok {
		return "", fmt.Errorf("filesystem is unavailable")
	}
	content, err := fsys.ReadFile(ctx, args.Path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", args.Path, err)
	}
	return marshalResult(struct {
		Content string `json:"content"`
	}{Content: string(content)})
}

func writeFile(
	ctx context.Context,
	env sandbox.Environment,
	raw json.RawMessage,
) (string, error) {
	var args writeArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("decode write_file arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("write_file path is required")
	}
	fsys, ok := env.FS()
	if !ok {
		return "", fmt.Errorf("filesystem is unavailable")
	}
	if err := fsys.WriteFile(ctx, args.Path, []byte(args.Content)); err != nil {
		return "", fmt.Errorf("write %q: %w", args.Path, err)
	}
	return marshalResult(struct {
		Path string `json:"path"`
	}{Path: args.Path})
}

func marshalResult(value any) (string, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode tool result: %w", err)
	}
	return string(result), nil
}
