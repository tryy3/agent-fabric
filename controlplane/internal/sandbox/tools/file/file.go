package file

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type readArgs struct {
	Path string `json:"path"`
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// MutatingTools names the tools that change the project filesystem. The agent
// uses it to decide when to auto-commit and the client when to refresh the
// Workbench tree.
var MutatingTools = []string{
	"write_file",
	"apply_patch",
	"append_file",
	"create_directory",
	"move_path",
	"delete_path",
}

// IsMutating reports whether name is a filesystem-mutating tool.
func IsMutating(name string) bool {
	return slices.Contains(MutatingTools, name)
}

const pathDescription = "Path under the workspace root (absolute within root, or relative)"

func Tools() []sandbox.Tool {
	requiresFS := sandbox.Capabilities{FS: true}
	return []sandbox.Tool{
		{
			Name:        "read_file",
			Description: "Read a text file in the workspace. Files over 2 MiB and binary files are rejected.",
			Parameters: sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"path": {Type: "string", Description: pathDescription},
				},
				Required: []string{"path"},
			},
			Requires: requiresFS,
			Run:      readFile,
		},
		{
			Name:        "write_file",
			Description: "Write content to a file in the workspace, creating parent directories. Replaces the whole file; prefer apply_patch or append_file for small edits.",
			Parameters: sandbox.Parameters{
				Properties: map[string]sandbox.Property{
					"path":    {Type: "string", Description: pathDescription},
					"content": {Type: "string", Description: "File content to write"},
				},
				Required: []string{"path", "content"},
			},
			Requires: requiresFS,
			Run:      writeFile,
		},
		listFilesTool(requiresFS),
		searchTextTool(requiresFS),
		applyPatchTool(requiresFS),
		appendFileTool(requiresFS),
		createDirectoryTool(requiresFS),
		movePathTool(requiresFS),
		deletePathTool(requiresFS),
	}
}

// decodeArgs unmarshals tool arguments, returning a ready coded failure when
// they are malformed.
func decodeArgs(raw json.RawMessage, tool string, into any) (failure string, err error) {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if decodeErr := json.Unmarshal(raw, into); decodeErr != nil {
		return fail(codeInvalidArgs, "decode %s arguments: %v", tool, decodeErr)
	}
	return "", nil
}

// openFS returns the environment filesystem or a coded failure.
func openFS(env sandbox.Environment) (sandbox.FS, string, error) {
	fsys, ok := env.FS()
	if !ok {
		res, err := fail(codeIO, "filesystem is unavailable")
		return nil, res, err
	}
	return fsys, "", nil
}

func readFile(
	ctx context.Context,
	env sandbox.Environment,
	raw json.RawMessage,
) (string, error) {
	var args readArgs
	if failure, err := decodeArgs(raw, "read_file", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.Path == "" {
		return fail(codeInvalidArgs, "read_file path is required")
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	content, failure, err := readText(ctx, fsys, args.Path, "read_file")
	if failure != "" || err != nil {
		return failure, err
	}
	return marshalResult(struct {
		Content string `json:"content"`
	}{Content: content})
}

func writeFile(
	ctx context.Context,
	env sandbox.Environment,
	raw json.RawMessage,
) (string, error) {
	var args writeArgs
	if failure, err := decodeArgs(raw, "write_file", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.Path == "" {
		return fail(codeInvalidArgs, "write_file path is required")
	}
	if isProtected(args.Path) {
		return fail(codeProtected, "write_file: %q is protected", args.Path)
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	if err := fsys.WriteFile(ctx, args.Path, []byte(args.Content)); err != nil {
		return failFS("write_file", args.Path, err)
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
