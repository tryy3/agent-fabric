// Package skills discovers, layers and toggles skills: folders with a SKILL.md
// that the agent loads on demand.
package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Source names the layer a skill came from. Later layers win over earlier ones.
type Source string

const (
	SourceBuiltin   Source = "builtin"
	SourcePlane     Source = "plane"
	SourceProject   Source = "project"
	SourceAssistant Source = "assistant"
)

// Requires values for the `requires` frontmatter key.
const (
	RequiresNone    = "none"
	RequiresSandbox = "sandbox"
)

// Skill is one parsed skill folder.
type Skill struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	ArgumentHint  string   `json:"argumentHint,omitempty"`
	Flags         []string `json:"flags,omitempty"`
	MaxToolRounds int      `json:"maxToolRounds,omitempty"`
	Requires      string   `json:"requires"`
	Source        Source   `json:"source"`
	// Hash is the hex SHA-256 of SKILL.md and the sorted extra-file paths.
	Hash string `json:"hash"`
	// Files lists the extra files in the folder, relative to it.
	Files []string `json:"files,omitempty"`
	// Body is the SKILL.md text after the frontmatter.
	Body string `json:"-"`
	// Path is the skill folder inside its source.
	Path string `json:"path,omitempty"`
}

// Entry is a skill in the effective list together with its toggle state.
type Entry struct {
	Skill
	Enabled bool `json:"enabled"`
	// Overrides is the source of the lower-layer skill this one replaced.
	Overrides []Source `json:"overrides,omitempty"`
}

// Pinned is the part of a resolved skill that is pinned at session/new.
type Pinned struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      Source `json:"source"`
	Hash        string `json:"hash"`
}

// Problem reports a skill folder that was skipped.
type Problem struct {
	Source Source `json:"source"`
	Path   string `json:"path"`
	Error  string `json:"error"`
}

func hashSkill(raw []byte, files []string) string {
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	h := sha256.New()
	h.Write(raw)
	for _, f := range sorted {
		h.Write([]byte{0})
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}
