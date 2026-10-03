package file

import (
	"context"
	"encoding/json"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type listArgs struct {
	Path           string `json:"path"`
	Recursive      bool   `json:"recursive"`
	MaxDepth       int    `json:"max_depth"`
	Glob           string `json:"glob"`
	IncludeIgnored bool   `json:"include_ignored"`
}

type listEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

func listFilesTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name: "list_files",
		Description: "List files and directories under a project directory, sorted by path. " +
			"By default lists one level; set recursive for a bounded tree. Generated directories " +
			"(.git, node_modules, dist, build, ...) are skipped unless include_ignored is set. " +
			"Results are capped at 1000 entries; check truncated.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"path":      {Type: "string", Description: "Directory to list (default: project root)"},
				"recursive": {Type: "boolean", Description: "Descend into subdirectories (default false)"},
				"max_depth": {Type: "integer", Description: "Maximum depth when recursive, 1-8 (default 8)"},
				"glob": {
					Type:        "string",
					Description: "Only return entries matching this glob (* ? **); a pattern without / matches the base name",
				},
				"include_ignored": {Type: "boolean", Description: "Also list ignored/generated directories (default false)"},
			},
		},
		Requires: requiresFS,
		Run:      listFiles,
	}
}

func listFiles(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args listArgs
	if failure, err := decodeArgs(raw, "list_files", &args); failure != "" || err != nil {
		return failure, err
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	match, globErr := globMatcher(args.Glob)
	if globErr != nil {
		return fail(codeInvalidArgs, "list_files: %v", globErr)
	}
	dir := cleanRel(args.Path)
	info, statErr := fsys.Stat(ctx, dir)
	if statErr != nil {
		return failFS("list_files", dir, statErr)
	}
	if !info.IsDir() {
		return fail(codeNotDir, "list_files: %q is not a directory", dir)
	}

	opts := walkOptions{MaxDepth: 1, IncludeIgnored: args.IncludeIgnored}
	if args.Recursive {
		opts.MaxDepth = MaxListDepth
		if args.MaxDepth > 0 && args.MaxDepth < MaxListDepth {
			opts.MaxDepth = args.MaxDepth
		}
	}

	entries := []listEntry{}
	truncated, reason := false, "max_entries"
	walkErr := walk(ctx, fsys, dir, opts, func(e walkEntry) error {
		if !match(e.Rel) {
			return nil
		}
		if len(entries) >= MaxListEntries {
			truncated = true
			return errStopWalk
		}
		entry := listEntry{Path: e.Rel, Type: "file", Size: e.Size}
		if e.IsDir {
			entry.Type, entry.Size = "dir", 0
		}
		entries = append(entries, entry)
		return nil
	})
	switch {
	case walkErr == nil || walkErr == errStopWalk:
	case walkErr == errWalkLimit:
		truncated, reason = true, "max_walk"
	default:
		return failFS("list_files", dir, walkErr)
	}

	result := struct {
		Path      string      `json:"path"`
		Entries   []listEntry `json:"entries"`
		Count     int         `json:"count"`
		Truncated bool        `json:"truncated"`
		Reason    string      `json:"truncated_reason,omitempty"`
	}{Path: dir, Entries: entries, Count: len(entries), Truncated: truncated}
	if truncated {
		result.Reason = reason
	}
	return marshalResult(result)
}
