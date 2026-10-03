// Package udiff parses and applies unified diffs with exact-match semantics:
// context and removed lines must equal the file at the stated position, and
// there is no fuzzy matching or offset search.
//
// It is shared by the apply_patch tool and the tool Gate, which needs the
// target paths before the tool runs.
package udiff

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrMismatch is wrapped when a hunk does not match the file content.
var ErrMismatch = errors.New("hunk does not match file")

// ErrMixedLineEndings is returned for files mixing CRLF and LF line endings.
var ErrMixedLineEndings = errors.New("file mixes CRLF and LF line endings")

// ErrUnsupported is wrapped for diffs that delete or rename files.
var ErrUnsupported = errors.New("unsupported diff operation")

// File is one file section of a diff.
type File struct {
	// Path is the project-relative target path (git a/ b/ prefixes removed).
	Path string
	// Create is true when the diff creates the file (--- /dev/null).
	Create bool
	Hunks  []Hunk
}

// Hunk is one @@ block. OldStart is the 1-based line of the first old line.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []Line
}

// Line is one hunk body line. Op is ' ', '-' or '+'.
type Line struct {
	Op   byte
	Text string
	// NoEOL is set when the line is followed by "\ No newline at end of file".
	NoEOL bool
}

const devNull = "/dev/null"

// Parse reads a unified diff into per-file sections.
func Parse(diff string) ([]File, error) {
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	// A trailing newline yields one empty final element; drop it.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	var files []File
	seen := map[string]bool{}
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--- "):
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
				return nil, fmt.Errorf("line %d: %q must be followed by a +++ header", i+1, line)
			}
			file, next, err := parseFile(lines, i)
			if err != nil {
				return nil, err
			}
			if seen[file.Path] {
				return nil, fmt.Errorf("file %q appears more than once in the diff", file.Path)
			}
			seen[file.Path] = true
			files = append(files, file)
			i = next
		case strings.HasPrefix(line, "rename ") || strings.HasPrefix(line, "deleted file"):
			return nil, fmt.Errorf("%w: %q (use move_path or delete_path)", ErrUnsupported, line)
		case len(files) > 0 && strings.TrimSpace(line) != "" && strings.ContainsRune(" +-", rune(line[0])):
			// Body lines left over after a hunk's declared counts were
			// satisfied. Skipping them would apply a silently truncated
			// patch, so a miscounted hunk header is an error.
			return nil, fmt.Errorf("line %d: %q follows a complete hunk; the hunk header counts are too small", i+1, line)
		default:
			// Preamble: "diff --git", "index", blank lines, prose.
			i++
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no file sections found; expected --- / +++ headers and @@ hunks")
	}
	return files, nil
}

func parseFile(lines []string, i int) (File, int, error) {
	oldPath := headerPath(lines[i][4:])
	newPath := headerPath(lines[i+1][4:])
	i += 2
	var file File
	switch {
	case newPath == devNull:
		return File{}, 0, fmt.Errorf("%w: deleting %q (use delete_path)", ErrUnsupported, oldPath)
	case oldPath == devNull:
		file.Create = true
		file.Path = strings.TrimPrefix(newPath, "b/")
	default:
		if strings.HasPrefix(oldPath, "a/") && strings.HasPrefix(newPath, "b/") {
			oldPath, newPath = oldPath[2:], newPath[2:]
		}
		if oldPath != newPath {
			return File{}, 0, fmt.Errorf("%w: renaming %q to %q (use move_path)", ErrUnsupported, oldPath, newPath)
		}
		file.Path = newPath
	}
	if file.Path == "" {
		return File{}, 0, errors.New("empty file path in diff header")
	}
	for i < len(lines) && strings.HasPrefix(lines[i], "@@") {
		hunk, next, err := parseHunk(lines, i)
		if err != nil {
			return File{}, 0, fmt.Errorf("%s: %w", file.Path, err)
		}
		file.Hunks = append(file.Hunks, hunk)
		i = next
	}
	if len(file.Hunks) == 0 {
		return File{}, 0, fmt.Errorf("%s: no @@ hunks", file.Path)
	}
	return file, i, nil
}

// headerPath drops a trailing tab-separated timestamp from a ---/+++ header.
func headerPath(s string) string {
	if idx := strings.IndexByte(s, '\t'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func parseHunk(lines []string, i int) (Hunk, int, error) {
	var h Hunk
	header := lines[i]
	if err := parseHunkHeader(header, &h); err != nil {
		return Hunk{}, 0, err
	}
	i++
	oldSeen, newSeen := 0, 0
	for (oldSeen < h.OldCount || newSeen < h.NewCount) && i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, `\`) {
			return Hunk{}, 0, fmt.Errorf("unexpected %q", line)
		}
		// An empty line is an empty context line whose leading space was
		// stripped by an editor.
		op, text := byte(' '), ""
		if line != "" {
			op, text = line[0], line[1:]
		}
		switch op {
		case ' ':
			oldSeen++
			newSeen++
		case '-':
			oldSeen++
		case '+':
			newSeen++
		default:
			return Hunk{}, 0, fmt.Errorf("line %d: %q must start with ' ', '-' or '+'", i+1, line)
		}
		h.Lines = append(h.Lines, Line{Op: op, Text: text})
		i++
		if i < len(lines) && strings.HasPrefix(lines[i], `\`) {
			h.Lines[len(h.Lines)-1].NoEOL = true
			i++
		}
	}
	if oldSeen != h.OldCount || newSeen != h.NewCount {
		return Hunk{}, 0, fmt.Errorf(
			"hunk %q declares -%d +%d lines but contains -%d +%d",
			header, h.OldCount, h.NewCount, oldSeen, newSeen,
		)
	}
	return h, i, nil
}

// parseHunkHeader parses "@@ -a[,b] +c[,d] @@ optional section".
func parseHunkHeader(header string, h *Hunk) error {
	bad := fmt.Errorf("malformed hunk header %q", header)
	rest := strings.TrimPrefix(header, "@@")
	end := strings.Index(rest, "@@")
	if end < 0 {
		return bad
	}
	fields := strings.Fields(rest[:end])
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "-") || !strings.HasPrefix(fields[1], "+") {
		return bad
	}
	var err error
	if h.OldStart, h.OldCount, err = parseRange(fields[0][1:]); err != nil {
		return bad
	}
	if h.NewStart, h.NewCount, err = parseRange(fields[1][1:]); err != nil {
		return bad
	}
	return nil
}

func parseRange(s string) (start, count int, err error) {
	count = 1
	startText, countText, hasCount := strings.Cut(s, ",")
	if start, err = strconv.Atoi(startText); err != nil {
		return 0, 0, err
	}
	if hasCount {
		if count, err = strconv.Atoi(countText); err != nil {
			return 0, 0, err
		}
	}
	if start < 0 || count < 0 {
		return 0, 0, errors.New("negative range")
	}
	return start, count, nil
}

// applyLF is Apply for LF-only content. For
// a created file original must be empty. Hunks must be ordered and must match
// the original exactly at their stated OldStart.
func applyLF(original string, f File) (string, error) {
	var src []string
	hadEOL := true
	if original != "" {
		src = strings.Split(original, "\n")
		if src[len(src)-1] == "" {
			src = src[:len(src)-1]
		} else {
			hadEOL = false
		}
	}
	if f.Create && len(src) > 0 {
		return "", fmt.Errorf("%w: %s already has content", ErrMismatch, f.Path)
	}

	out := make([]string, 0, len(src)+16)
	pos := 0 // next unconsumed index in src
	noEOLNew, noEOLOldOnly := false, false
	for n, h := range f.Hunks {
		start := h.OldStart - 1
		if h.OldCount == 0 {
			// Pure insertion: OldStart is the line after which to insert.
			start = h.OldStart
		}
		if start < pos {
			return "", fmt.Errorf("hunk %d overlaps or precedes the previous hunk", n+1)
		}
		if start > len(src) {
			return "", fmt.Errorf("%w: hunk %d starts at line %d but %s has %d lines",
				ErrMismatch, n+1, h.OldStart, f.Path, len(src))
		}
		out = append(out, src[pos:start]...)
		cursor := start
		for _, l := range h.Lines {
			switch l.Op {
			case ' ', '-':
				if cursor >= len(src) || src[cursor] != l.Text {
					return "", mismatch(f.Path, n+1, cursor, src, l.Text)
				}
				cursor++
				if l.Op == ' ' {
					out = append(out, l.Text)
				}
				if l.NoEOL && l.Op == ' ' {
					noEOLNew = true
				}
				if l.NoEOL && l.Op == '-' {
					noEOLOldOnly = true
				}
			case '+':
				out = append(out, l.Text)
				if l.NoEOL {
					noEOLNew = true
				}
			}
		}
		pos = cursor
	}
	out = append(out, src[pos:]...)

	eol := hadEOL
	switch {
	case noEOLNew:
		eol = false
	case noEOLOldOnly:
		eol = true
	}
	result := strings.Join(out, "\n")
	if eol && len(out) > 0 {
		result += "\n"
	}
	return result, nil
}

func mismatch(path string, hunk, cursor int, src []string, want string) error {
	got := "<end of file>"
	if cursor < len(src) {
		got = strconv.Quote(src[cursor])
	}
	return fmt.Errorf("%w: %s hunk %d line %d: expected %s, file has %s",
		ErrMismatch, path, hunk, cursor+1, strconv.Quote(want), got)
}

// Apply applies one file's hunks to original and returns the new content. For
// a created file original must be empty. Hunks must be ordered and must match
// the original exactly at their stated OldStart.
//
// A file that uses CRLF throughout is patched as LF and written back as CRLF.
// A file mixing CRLF and bare LF is refused with ErrMixedLineEndings rather
// than silently normalized.
func Apply(original string, f File) (string, error) {
	crlf := strings.Count(original, "\r\n")
	if crlf == 0 {
		return applyLF(original, f)
	}
	if crlf != strings.Count(original, "\n") {
		return "", fmt.Errorf("%w: %s (use write_file)", ErrMixedLineEndings, f.Path)
	}
	out, err := applyLF(strings.ReplaceAll(original, "\r\n", "\n"), f)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(out, "\n", "\r\n"), nil
}
