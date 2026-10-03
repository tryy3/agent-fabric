package file

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// Named limits. They are deliberately not part of any tool schema so tuning
// them later does not change what the model sees.
const (
	// MaxEditFileBytes matches the Workbench editor limit (2 MiB). It bounds
	// read_file, apply_patch targets, and append_file results.
	MaxEditFileBytes = 2 << 20

	MaxListEntries = 1000
	MaxListDepth   = 8

	// MaxWalkEntries bounds directory entries visited by one list_files or
	// search_text call regardless of filters; on Docker each directory is an
	// exec, so a non-matching glob must not walk an unbounded tree.
	MaxWalkEntries = 20000

	MaxSearchFiles        = 1000
	MaxSearchBytes        = 32 << 20
	MaxSearchMatchesTotal = 200
	MaxSearchMatchesFile  = 50
	MaxSearchOutputBytes  = 256 << 10
	MaxSearchLineChars    = 500

	binarySniffBytes = 8 << 10
)

// Stable machine-readable error codes returned in {"error","code"}.
const (
	codeNotFound    = "not_found"
	codeExists      = "exists"
	codeTooLarge    = "too_large"
	codeBinary      = "binary"
	codeMismatch    = "mismatch"
	codeNotEmpty    = "not_empty"
	codeProtected   = "protected"
	codeInvalidArgs = "invalid_args"
	codeNotDir      = "not_dir"
	codeIsDir       = "is_dir"
	codeUnsupported = "unsupported"
	codeIO          = "io_error"
)

// defaultIgnoredDirs are skipped by list_files and search_text unless the
// caller sets include_ignored. The set is fixed so behavior is identical in
// every environment and does not depend on git or a .gitignore parser.
var defaultIgnoredDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"dist":          true,
	"build":         true,
	".dart_tool":    true,
	".next":         true,
	".cache":        true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	"target":        true,
	"vendor":        true,
	".idea":         true,
	".gradle":       true,
	"coverage":      true,
	".pytest_cache": true,
}

// fail encodes a model-facing error with a stable code. It is returned as a
// normal result (not a Go error) so the code field survives; the agent treats
// any result with a top-level "error" as failed.
func fail(code, format string, args ...any) (string, error) {
	return marshalResult(struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}{Error: fmt.Sprintf(format, args...), Code: code})
}

// failFS maps a filesystem error to a coded failure. Context cancellation is
// returned as a Go error so the agent aborts the turn instead of feeding the
// model a bogus io_error result.
func failFS(op, p string, err error) (string, error) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "", err
	case errors.Is(err, fs.ErrNotExist):
		return fail(codeNotFound, "%s: %q does not exist", op, p)
	case errors.Is(err, fs.ErrExist):
		return fail(codeExists, "%s: %q already exists", op, p)
	case errors.Is(err, fs.ErrPermission):
		return fail(codeProtected, "%s %q: %v", op, p, err)
	}
	return fail(codeIO, "%s %q: %v", op, p, err)
}

// isProtected reports whether a project-relative path is repository metadata
// the agent may never mutate. The Gate enforces the same rule on resolved
// paths; this is defense in depth for callers that bypass the Gate.
func isProtected(p string) bool {
	cleaned := path.Clean(strings.TrimLeft(strings.TrimSpace(p), "/"))
	return cleaned == ".git" || strings.HasPrefix(cleaned, ".git/")
}

// looksBinary reports NUL bytes in the first sniff window or invalid UTF-8.
func looksBinary(data []byte) bool {
	head := data
	if len(head) > binarySniffBytes {
		head = head[:binarySniffBytes]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}
	// A multi-byte rune can be cut at the window edge; forgive up to
	// UTFMax-1 trailing bytes when the data was truncated.
	for i := 0; i < utf8.UTFMax; i++ {
		if utf8.Valid(head) {
			return false
		}
		if len(data) == len(head) || len(head) == 0 {
			return true
		}
		head = head[:len(head)-1]
	}
	return true
}

// readText reads a text file within the edit ceiling. A non-empty failure is
// a ready-to-return coded error result.
func readText(ctx context.Context, fsys sandbox.FS, p, op string) (content string, failure string, err error) {
	info, statErr := fsys.Stat(ctx, p)
	if statErr != nil {
		res, e := failFS(op, p, statErr)
		return "", res, e
	}
	if info.IsDir() {
		res, e := fail(codeIsDir, "%s: %q is a directory", op, p)
		return "", res, e
	}
	if info.Size() > MaxEditFileBytes {
		res, e := fail(codeTooLarge, "%s: %q is %d bytes, over the %d byte limit", op, p, info.Size(), MaxEditFileBytes)
		return "", res, e
	}
	data, readErr := fsys.ReadFile(ctx, p)
	if readErr != nil {
		res, e := failFS(op, p, readErr)
		return "", res, e
	}
	if looksBinary(data) {
		res, e := fail(codeBinary, "%s: %q looks like a binary file", op, p)
		return "", res, e
	}
	return string(data), "", nil
}

// walkEntry is one visited path. Rel is project-relative with forward slashes.
type walkEntry struct {
	Rel   string
	Name  string
	IsDir bool
	Size  int64
	Depth int
}

type walkOptions struct {
	// MaxDepth limits descent below the start directory (1 = direct children).
	MaxDepth       int
	IncludeIgnored bool
}

// errStopWalk ends a walk early without being an error.
var errStopWalk = errors.New("stop walk")

// errWalkLimit ends a walk that visited MaxWalkEntries entries.
var errWalkLimit = errors.New("walk entry limit")

// walk visits entries under dir depth-first in lexical order, directories
// before their children. It is built on FS.ReadDir only so local and
// exec-backed filesystems traverse identically. visit may return errStopWalk.
func walk(ctx context.Context, fsys sandbox.FS, dir string, opts walkOptions, visit func(walkEntry) error) error {
	visited := 0
	return walkDir(ctx, fsys, cleanRel(dir), 1, &visited, opts, visit)
}

func walkDir(ctx context.Context, fsys sandbox.FS, dir string, depth int, visited *int, opts walkOptions, visit func(walkEntry) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		// Only the start directory's failure is the caller's error; a nested
		// directory that vanished or is unreadable must not discard the rest
		// of the walk.
		if depth > 1 && ctx.Err() == nil && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission)) {
			return nil
		}
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	for _, e := range entries {
		if e.IsDir && !opts.IncludeIgnored && defaultIgnoredDirs[e.Name] {
			continue
		}
		if *visited++; *visited > MaxWalkEntries {
			return errWalkLimit
		}
		rel := e.Name
		if dir != "." {
			rel = dir + "/" + e.Name
		}
		if err := visit(walkEntry{Rel: rel, Name: e.Name, IsDir: e.IsDir, Size: e.Size, Depth: depth}); err != nil {
			return err
		}
		if e.IsDir && depth < opts.MaxDepth {
			if err := walkDir(ctx, fsys, rel, depth+1, visited, opts, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

// cleanRel normalizes an optional project-relative path; empty means ".".
func cleanRel(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "."
	}
	return path.Clean(p)
}

// globMatcher compiles a glob supporting *, ?, and ** (any depth). A pattern
// without "/" matches the base name; otherwise it matches the full path.
func globMatcher(pattern string) (func(rel string) bool, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return func(string) bool { return true }, nil
	}
	baseOnly := !strings.Contains(pattern, "/")
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	return func(rel string) bool {
		if baseOnly {
			rel = path.Base(rel)
		}
		return re.MatchString(rel)
	}, nil
}
