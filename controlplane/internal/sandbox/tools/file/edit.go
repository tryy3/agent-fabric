package file

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file/udiff"
	"github.com/tryy3/agent-fabric/internal/workspace/fsops"
)

// ---- apply_patch ----------------------------------------------------------

type patchArgs struct {
	Diff string `json:"diff"`
}

type patchedFile struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Hunks  int    `json:"hunks"`
}

func applyPatchTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name: "apply_patch",
		Description: "Apply a unified diff to project files. Context and removed lines must match the file " +
			"exactly at the stated line numbers (no fuzzy matching). All files are validated before any is " +
			"written; on any mismatch nothing changes. Supports editing existing files and creating files " +
			"(--- /dev/null). Git-style a/ b/ path prefixes are stripped. Use delete_path or move_path to " +
			"delete or rename. Files over 2 MiB and binary files are rejected.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"diff": {Type: "string", Description: "Unified diff with ---/+++ headers and @@ -a,b +c,d @@ hunks"},
			},
			Required: []string{"diff"},
		},
		Requires: requiresFS,
		Run:      applyPatch,
	}
}

func applyPatch(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args patchArgs
	if failure, err := decodeArgs(raw, "apply_patch", &args); failure != "" || err != nil {
		return failure, err
	}
	if strings.TrimSpace(args.Diff) == "" {
		return fail(codeInvalidArgs, "apply_patch diff is required")
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	files, parseErr := udiff.Parse(args.Diff)
	if parseErr != nil {
		code := codeInvalidArgs
		if errors.Is(parseErr, udiff.ErrUnsupported) {
			code = codeUnsupported
		}
		return fail(code, "apply_patch: %v", parseErr)
	}

	type staged struct {
		file     udiff.File
		original string
		updated  string
	}
	plan := make([]staged, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		if isProtected(f.Path) {
			return fail(codeProtected, "apply_patch: %q is protected", f.Path)
		}
		// The parser dedups on the literal header path; "a.go" and "./a.go"
		// would otherwise stage two patches against one original and the
		// later write would silently clobber the earlier one.
		key := cleanRel(strings.TrimLeft(f.Path, "/"))
		if seen[key] {
			return fail(codeInvalidArgs, "apply_patch: file %q appears more than once in the diff", f.Path)
		}
		seen[key] = true
		var original string
		if f.Create {
			if _, statErr := fsys.Stat(ctx, f.Path); statErr == nil {
				return fail(codeExists, "apply_patch: %q already exists", f.Path)
			} else if !errors.Is(statErr, fs.ErrNotExist) {
				return failFS("apply_patch", f.Path, statErr)
			}
		} else {
			content, failure, err := readText(ctx, fsys, f.Path, "apply_patch")
			if failure != "" || err != nil {
				return failure, err
			}
			original = content
		}
		updated, applyErr := udiff.Apply(original, f)
		if applyErr != nil {
			code := codeInvalidArgs
			if errors.Is(applyErr, udiff.ErrMismatch) {
				code = codeMismatch
			} else if errors.Is(applyErr, udiff.ErrMixedLineEndings) {
				code = codeUnsupported
			}
			return fail(code, "apply_patch: %v (no files were changed)", applyErr)
		}
		if len(updated) > MaxEditFileBytes {
			return fail(codeTooLarge, "apply_patch: result for %q would be %d bytes, over the %d byte limit",
				f.Path, len(updated), MaxEditFileBytes)
		}
		plan = append(plan, staged{file: f, original: original, updated: updated})
	}

	// Everything validated; write, restoring earlier files if a write fails.
	for i, s := range plan {
		if writeErr := fsys.WriteFile(ctx, s.file.Path, []byte(s.updated)); writeErr != nil {
			rollback := context.WithoutCancel(ctx)
			for _, done := range plan[:i] {
				if done.file.Create {
					_ = fsys.Remove(rollback, done.file.Path)
				} else {
					_ = fsys.WriteFile(rollback, done.file.Path, []byte(done.original))
				}
			}
			return failFS("apply_patch", s.file.Path, writeErr)
		}
	}

	results := make([]patchedFile, 0, len(plan))
	for _, s := range plan {
		action := "modified"
		if s.file.Create {
			action = "created"
		}
		results = append(results, patchedFile{Path: s.file.Path, Action: action, Hunks: len(s.file.Hunks)})
	}
	return marshalResult(struct {
		Files []patchedFile `json:"files"`
	}{Files: results})
}

// ---- append_file ----------------------------------------------------------

type appendArgs struct {
	Path          string `json:"path"`
	Content       string `json:"content"`
	EnsureNewline *bool  `json:"ensure_newline"`
	Create        bool   `json:"create"`
}

func appendFileTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name: "append_file",
		Description: "Append text to the end of an existing text file. With ensure_newline (default true) a " +
			"newline is inserted first if the file does not already end with one; content is otherwise " +
			"appended exactly as given (no trailing newline is added). The resulting file may not exceed 2 MiB.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"path":           {Type: "string", Description: pathDescription},
				"content":        {Type: "string", Description: "Text to append"},
				"ensure_newline": {Type: "boolean", Description: "Insert a newline before content if the file lacks a final newline (default true)"},
				"create":         {Type: "boolean", Description: "Create the file when it does not exist (default false)"},
			},
			Required: []string{"path", "content"},
		},
		Requires: requiresFS,
		Run:      appendFile,
	}
}

func appendFile(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args appendArgs
	if failure, err := decodeArgs(raw, "append_file", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.Path == "" {
		return fail(codeInvalidArgs, "append_file path is required")
	}
	if isProtected(args.Path) {
		return fail(codeProtected, "append_file: %q is protected", args.Path)
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}

	existing, created := "", false
	if _, statErr := fsys.Stat(ctx, args.Path); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return failFS("append_file", args.Path, statErr)
		}
		if !args.Create {
			return fail(codeNotFound, "append_file: %q does not exist (set create to create it)", args.Path)
		}
		created = true
	} else {
		content, failure, err := readText(ctx, fsys, args.Path, "append_file")
		if failure != "" || err != nil {
			return failure, err
		}
		existing = content
	}

	separator := ""
	ensure := args.EnsureNewline == nil || *args.EnsureNewline
	if ensure && existing != "" && !strings.HasSuffix(existing, "\n") {
		separator = "\n"
	}
	updated := existing + separator + args.Content
	if len(updated) > MaxEditFileBytes {
		return fail(codeTooLarge, "append_file: result for %q would be %d bytes, over the %d byte limit",
			args.Path, len(updated), MaxEditFileBytes)
	}
	if err := fsys.WriteFile(ctx, args.Path, []byte(updated)); err != nil {
		return failFS("append_file", args.Path, err)
	}
	return marshalResult(struct {
		Path          string `json:"path"`
		AppendedBytes int    `json:"appended_bytes"`
		Size          int    `json:"size"`
		Created       bool   `json:"created"`
	}{Path: args.Path, AppendedBytes: len(separator) + len(args.Content), Size: len(updated), Created: created})
}

// ---- create_directory -----------------------------------------------------

type pathArgs struct {
	Path string `json:"path"`
}

func createDirectoryTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name:        "create_directory",
		Description: "Create a directory, including missing parents. Succeeds without change if it already exists as a directory.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{"path": {Type: "string", Description: pathDescription}},
			Required:   []string{"path"},
		},
		Requires: requiresFS,
		Run:      createDirectory,
	}
}

func createDirectory(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args pathArgs
	if failure, err := decodeArgs(raw, "create_directory", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.Path == "" {
		return fail(codeInvalidArgs, "create_directory path is required")
	}
	if isProtected(args.Path) {
		return fail(codeProtected, "create_directory: %q is protected", args.Path)
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	created := true
	if info, statErr := fsys.Stat(ctx, args.Path); statErr == nil {
		if !info.IsDir() {
			return fail(codeExists, "create_directory: %q exists and is not a directory", args.Path)
		}
		created = false
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return failFS("create_directory", args.Path, statErr)
	}
	if created {
		if err := fsys.Mkdir(ctx, args.Path); err != nil {
			return failFS("create_directory", args.Path, err)
		}
	}
	return marshalResult(struct {
		Path    string `json:"path"`
		Created bool   `json:"created"`
	}{Path: args.Path, Created: created})
}

// ---- move_path ------------------------------------------------------------

type moveArgs struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func movePathTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name: "move_path",
		Description: "Rename or move a file or directory. Never overwrites: fails if the destination exists. " +
			"The destination's parent directory must already exist (use create_directory first).",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"from": {Type: "string", Description: "Existing project-relative path"},
				"to":   {Type: "string", Description: "New project-relative path"},
			},
			Required: []string{"from", "to"},
		},
		Requires: requiresFS,
		Run:      movePath,
	}
}

func movePath(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args moveArgs
	if failure, err := decodeArgs(raw, "move_path", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.From == "" || args.To == "" {
		return fail(codeInvalidArgs, "move_path from and to are required")
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	dst, moveErr := fsops.New(fsys, fsops.WithGuard(fsops.ProtectGit)).Move(ctx, args.From, args.To)
	if moveErr != nil {
		switch {
		case errors.Is(moveErr, fsops.ErrProtected):
			return fail(codeProtected, "move_path: %v", moveErr)
		case errors.Is(moveErr, fsops.ErrNotFound):
			return fail(codeNotFound, "move_path: %v", moveErr)
		case errors.Is(moveErr, fsops.ErrExists):
			return fail(codeExists, "move_path: %v (nothing was overwritten)", moveErr)
		case errors.Is(moveErr, fsops.ErrInvalidPath),
			errors.Is(moveErr, fsops.ErrIntoSelf),
			errors.Is(moveErr, fsops.ErrEscapesRoot):
			return fail(codeInvalidArgs, "move_path: %v", moveErr)
		}
		return failFS("move_path", args.From, moveErr)
	}
	return marshalResult(struct {
		From string `json:"from"`
		To   string `json:"to"`
	}{From: args.From, To: dst})
}

// ---- delete_path ----------------------------------------------------------

func deletePathTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name: "delete_path",
		Description: "Delete a file or an empty directory. Non-empty directories are refused (delete their " +
			"contents first). The project root and .git cannot be deleted. Requires user confirmation.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{"path": {Type: "string", Description: pathDescription}},
			Required:   []string{"path"},
		},
		Requires: requiresFS,
		Run:      deletePath,
	}
}

func deletePath(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args pathArgs
	if failure, err := decodeArgs(raw, "delete_path", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.Path == "" {
		return fail(codeInvalidArgs, "delete_path path is required")
	}
	cleaned, normErr := fsops.Normalize(args.Path)
	if normErr != nil {
		return fail(codeInvalidArgs, "delete_path: %v", normErr)
	}
	if isProtected(cleaned) {
		return fail(codeProtected, "delete_path: %q is protected", args.Path)
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	info, statErr := fsys.Stat(ctx, cleaned)
	if statErr != nil {
		return failFS("delete_path", cleaned, statErr)
	}
	kind := "file"
	if info.IsDir() {
		kind = "dir"
		children, readErr := fsys.ReadDir(ctx, cleaned)
		if readErr != nil {
			return failFS("delete_path", cleaned, readErr)
		}
		if len(children) > 0 {
			return fail(codeNotEmpty, "delete_path: directory %q is not empty (%d entries); delete its contents first", cleaned, len(children))
		}
	}
	if err := fsys.Remove(ctx, cleaned); err != nil {
		return failFS("delete_path", cleaned, err)
	}
	return marshalResult(struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}{Path: cleaned, Type: kind})
}
