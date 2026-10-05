// Package gatebench scores gate setups (deterministic rules, LLM scorers,
// mixes of both) against a labelled dataset of tool calls. It drives the same
// gate.Chain and gate.Policy the live agent uses.
package gatebench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
)

// Case is one labelled tool call.
type Case struct {
	ID       string          `json:"id"`
	Category string          `json:"category"`
	Tool     string          `json:"tool"`
	Args     json.RawMessage `json:"args"`
	Context  Context         `json:"context"`
	Expect   Expect          `json:"expect"`
	// Rules documents what the deterministic rules are known to return when
	// that differs from Expect (rules cannot see intent). The rules-only
	// regression test asserts this; benchmark metrics still use Expect.
	Rules *RulesExpect `json:"rules,omitempty"`
	Tags  []string     `json:"tags,omitempty"`
	Note  string       `json:"note,omitempty"`
}

// Context is the situation a call is made in.
type Context struct {
	UserIntent  string   `json:"userIntent,omitempty"`
	EnvKind     string   `json:"envKind,omitempty"`     // default "docker"
	ProjectRoot string   `json:"projectRoot,omitempty"` // default "/workspace"
	Grants      []string `json:"grants,omitempty"`
	// Tainted marks a session that has read web content: the policies ask from
	// the elevated band up and the deep tier may not lower a score.
	Tainted bool `json:"tainted,omitempty"`
}

// Expect is the ideal outcome: the score a perfect gate gives and, optionally,
// explicit decisions per mode (default: derived from Score through the default
// policies).
type Expect struct {
	Score     int                             `json:"score"`
	Tolerance *int                            `json:"tolerance,omitempty"` // default 1
	Decisions map[gate.Mode]gate.DecisionKind `json:"decisions,omitempty"`
}

// RulesExpect is the known score of the deterministic rules.
type RulesExpect struct {
	Score int `json:"score"`
}

// Tol is the allowed score distance.
func (e Expect) Tol() int {
	if e.Tolerance != nil {
		return *e.Tolerance
	}
	return 1
}

// Band is the expected band.
func (e Expect) Band() gate.Band { return gate.BandOf(e.Score) }

// Policy is the policy a mode applies to the case.
func (c Case) Policy(m gate.Mode) gate.Policy {
	p := gate.DefaultPolicies.PolicyFor(m)
	if c.Context.Tainted {
		p = p.Tainted()
	}
	return p
}

// Decision is the expected outcome in a mode.
func (c Case) Decision(m gate.Mode) gate.DecisionKind {
	if d, ok := c.Expect.Decisions[m]; ok {
		return d
	}
	return DecisionFor(c.Policy(m), c.Expect.Score)
}

// DecisionFor maps a score to the outcome a policy gives it.
func DecisionFor(p gate.Policy, score int) gate.DecisionKind {
	switch {
	case score >= p.CancelAt:
		return gate.Deny
	case score >= p.AskAt:
		return gate.Ask
	default:
		return gate.Allow
	}
}

// Request builds the gate request for the case.
func (c Case) Request() gate.Request {
	env, root := c.Context.EnvKind, c.Context.ProjectRoot
	if env == "" {
		env = "docker"
	}
	if root == "" {
		root = "/workspace"
	}
	return gate.Request{
		ToolName:      c.Tool,
		Args:          c.Args,
		ProjectRoot:   root,
		POSIX:         env != "local",
		EnvKind:       env,
		CommandGrants: c.Context.Grants,
		UserIntent:    c.Context.UserIntent,
		Tainted:       c.Context.Tainted,
	}
}

// LoadCases reads every *.json file in dir of fsys, validates the cases and
// returns them sorted by ID.
func LoadCases(fsys fs.FS, dir string) ([]Case, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []Case
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var cases []Case
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cases); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		for _, c := range cases {
			if err := c.validate(); err != nil {
				return nil, fmt.Errorf("%s: case %q: %w", e.Name(), c.ID, err)
			}
			if prev, dup := seen[c.ID]; dup {
				return nil, fmt.Errorf("%s: duplicate case id %q (also in %s)", e.Name(), c.ID, prev)
			}
			seen[c.ID] = e.Name()
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c Case) validate() error {
	switch {
	case strings.TrimSpace(c.ID) == "":
		return fmt.Errorf("id is required")
	case c.Category == "":
		return fmt.Errorf("category is required")
	case c.Tool == "":
		return fmt.Errorf("tool is required")
	case !json.Valid(c.Args):
		return fmt.Errorf("args must be valid JSON")
	case c.Expect.Score < gate.MinRisk || c.Expect.Score > gate.MaxRisk:
		return fmt.Errorf("expect.score must be 1-10")
	}
	for m, k := range c.Expect.Decisions {
		if _, err := gate.ParseMode(string(m)); err != nil || m == "" {
			return fmt.Errorf("expect.decisions: bad mode %q", m)
		}
		if k != gate.Allow && k != gate.Ask && k != gate.Deny {
			return fmt.Errorf("expect.decisions[%s]: bad decision %q", m, k)
		}
	}
	return nil
}

// Filter keeps cases whose category is in cats (all when empty).
func Filter(cases []Case, cats []string) []Case {
	if len(cats) == 0 {
		return cases
	}
	var out []Case
	for _, c := range cases {
		for _, cat := range cats {
			if c.Category == cat {
				out = append(out, c)
				break
			}
		}
	}
	return out
}
