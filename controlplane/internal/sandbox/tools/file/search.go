package file

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type searchArgs struct {
	Query         string `json:"query"`
	Regex         bool   `json:"regex"`
	Path          string `json:"path"`
	Glob          string `json:"glob"`
	CaseSensitive *bool  `json:"case_sensitive"`
}

type searchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func searchTextTool(requiresFS sandbox.Capabilities) sandbox.Tool {
	return sandbox.Tool{
		Name: "search_text",
		Description: "Search text files in the project for a literal string or regular expression. " +
			"Binary and over-2-MiB files and generated directories (.git, node_modules, ...) are skipped. " +
			"Results are deterministic (path order) and capped; check truncated and narrow path or glob if set.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"query":          {Type: "string", Description: "Text to find (literal unless regex is true)"},
				"regex":          {Type: "boolean", Description: "Treat query as an RE2 regular expression (default false)"},
				"path":           {Type: "string", Description: "File or directory to search (default: project root)"},
				"glob":           {Type: "string", Description: "Only search files matching this glob (* ? **); a pattern without / matches the base name"},
				"case_sensitive": {Type: "boolean", Description: "Case-sensitive match (default true)"},
			},
			Required: []string{"query"},
		},
		Requires: requiresFS,
		Run:      searchText,
	}
}

func searchText(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	var args searchArgs
	if failure, err := decodeArgs(raw, "search_text", &args); failure != "" || err != nil {
		return failure, err
	}
	if args.Query == "" {
		return fail(codeInvalidArgs, "search_text query is required")
	}
	fsys, failure, err := openFS(env)
	if fsys == nil {
		return failure, err
	}
	caseSensitive := args.CaseSensitive == nil || *args.CaseSensitive
	lineMatches, compileErr := compileMatcher(args.Query, args.Regex, caseSensitive)
	if compileErr != nil {
		return fail(codeInvalidArgs, "search_text: invalid query: %v", compileErr)
	}
	globMatch, globErr := globMatcher(args.Glob)
	if globErr != nil {
		return fail(codeInvalidArgs, "search_text: %v", globErr)
	}

	start := cleanRel(args.Path)
	info, statErr := fsys.Stat(ctx, start)
	if statErr != nil {
		return failFS("search_text", start, statErr)
	}

	s := &searcher{
		ctx:       ctx,
		fsys:      fsys,
		matchLine: lineMatches,
		matches:   []searchMatch{},
	}
	var walkErr error
	if info.IsDir() {
		walkErr = walk(ctx, fsys, start, walkOptions{MaxDepth: 1 << 20}, func(e walkEntry) error {
			if e.IsDir || !globMatch(e.Rel) {
				return nil
			}
			return s.file(e.Rel, e.Size)
		})
	} else {
		walkErr = s.file(start, info.Size())
	}
	if walkErr != nil && walkErr != errStopWalk {
		return failFS("search_text", start, walkErr)
	}

	return marshalResult(struct {
		Query                string        `json:"query"`
		Matches              []searchMatch `json:"matches"`
		FilesScanned         int           `json:"files_scanned"`
		BytesScanned         int64         `json:"bytes_scanned"`
		SkippedBinary        int           `json:"skipped_binary"`
		SkippedLarge         int           `json:"skipped_large"`
		FilesWithMoreMatches int           `json:"files_with_more_matches"`
		Truncated            bool          `json:"truncated"`
		TruncatedReason      string        `json:"truncated_reason,omitempty"`
	}{
		Query:                args.Query,
		Matches:              s.matches,
		FilesScanned:         s.filesScanned,
		BytesScanned:         s.bytesScanned,
		SkippedBinary:        s.skippedBinary,
		SkippedLarge:         s.skippedLarge,
		FilesWithMoreMatches: s.filesCapped,
		Truncated:            s.truncated != "",
		TruncatedReason:      s.truncated,
	})
}

func compileMatcher(query string, isRegex, caseSensitive bool) (func(string) bool, error) {
	if isRegex {
		pattern := query
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}
	if caseSensitive {
		return func(line string) bool { return strings.Contains(line, query) }, nil
	}
	lowered := strings.ToLower(query)
	return func(line string) bool { return strings.Contains(strings.ToLower(line), lowered) }, nil
}

// searcher accumulates bounded search state across files.
type searcher struct {
	ctx       context.Context
	fsys      sandbox.FS
	matchLine func(string) bool

	matches       []searchMatch
	outputBytes   int
	filesScanned  int
	bytesScanned  int64
	skippedBinary int
	skippedLarge  int
	filesCapped   int
	truncated     string
}

// file scans one file, returning errStopWalk once a global cap is hit.
func (s *searcher) file(rel string, size int64) error {
	if size > MaxEditFileBytes {
		s.skippedLarge++
		return nil
	}
	if s.filesScanned >= MaxSearchFiles {
		s.truncated = "max_files"
		return errStopWalk
	}
	if s.bytesScanned+size > MaxSearchBytes {
		s.truncated = "max_bytes"
		return errStopWalk
	}
	data, err := s.fsys.ReadFile(s.ctx, rel)
	if err != nil {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		return nil // unreadable file: skip deterministically
	}
	s.filesScanned++
	s.bytesScanned += int64(len(data))
	if looksBinary(data) {
		s.skippedBinary++
		return nil
	}

	perFile := 0
	for i, line := range strings.Split(string(data), "\n") {
		if !s.matchLine(line) {
			continue
		}
		if perFile >= MaxSearchMatchesFile {
			s.filesCapped++
			break
		}
		if len(s.matches) >= MaxSearchMatchesTotal {
			s.truncated = "max_matches"
			return errStopWalk
		}
		m := searchMatch{Path: rel, Line: i + 1, Text: clipLine(line)}
		cost := len(m.Path) + len(m.Text) + 32
		if s.outputBytes+cost > MaxSearchOutputBytes {
			s.truncated = "max_output"
			return errStopWalk
		}
		s.outputBytes += cost
		s.matches = append(s.matches, m)
		perFile++
	}
	return nil
}

func clipLine(line string) string {
	line = strings.TrimRight(line, "\r")
	if len(line) <= MaxSearchLineChars {
		return line
	}
	runes := []rune(line)
	if len(runes) <= MaxSearchLineChars {
		return line
	}
	return string(runes[:MaxSearchLineChars]) + "…"
}
