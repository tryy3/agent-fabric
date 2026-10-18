package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Built-in budget defaults, used when neither the assistant nor the plane sets
// a value. Normal Prompts keep the long-standing 8 tool rounds.
const (
	DefaultToolRounds     = 8
	DefaultMaxToolRounds  = 100
	DefaultMaxTurnSeconds = 15 * 60

	maxLimitRounds  = 1000
	minLimitSeconds = 10
	maxLimitSeconds = 24 * 60 * 60
)

// Limits is the `limits` object of plane settings and assistant settings. A nil
// field is unset and inherits.
type Limits struct {
	// DefaultToolRounds is how many tool rounds a normal Prompt may use.
	DefaultToolRounds *int `json:"defaultToolRounds,omitempty"`
	// MaxToolRounds is the ceiling for any run, a skill run included.
	MaxToolRounds *int `json:"maxToolRounds,omitempty"`
	// MaxTurnSeconds is the wall-clock limit of one Prompt.
	MaxTurnSeconds *int `json:"maxTurnSeconds,omitempty"`
}

// DecodeLimits strictly decodes and validates a limits object.
func DecodeLimits(raw json.RawMessage) (Limits, error) {
	var l Limits
	if len(raw) == 0 || isJSONNull(raw) {
		return l, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return Limits{}, fmt.Errorf("limits: %w", err)
	}
	if err := l.Validate(); err != nil {
		return Limits{}, err
	}
	return l, nil
}

// Validate checks ranges and that the default does not pass the maximum.
func (l Limits) Validate() error {
	for name, v := range map[string]*int{"defaultToolRounds": l.DefaultToolRounds, "maxToolRounds": l.MaxToolRounds} {
		if v != nil && (*v < 1 || *v > maxLimitRounds) {
			return fmt.Errorf("limits.%s must be between 1 and %d", name, maxLimitRounds)
		}
	}
	if v := l.MaxTurnSeconds; v != nil && (*v < minLimitSeconds || *v > maxLimitSeconds) {
		return fmt.Errorf("limits.maxTurnSeconds must be between %d and %d", minLimitSeconds, maxLimitSeconds)
	}
	if l.DefaultToolRounds != nil && l.MaxToolRounds != nil && *l.DefaultToolRounds > *l.MaxToolRounds {
		return fmt.Errorf("limits.defaultToolRounds may not exceed limits.maxToolRounds")
	}
	return nil
}

// LimitsFromSettings reads the limits object inside a settings JSON blob
// leniently: a missing or malformed value means no overrides.
func LimitsFromSettings(settings json.RawMessage) Limits {
	if len(settings) == 0 || isJSONNull(settings) {
		return Limits{}
	}
	var wrapper struct {
		Limits json.RawMessage `json:"limits"`
	}
	if err := json.Unmarshal(settings, &wrapper); err != nil {
		return Limits{}
	}
	l, err := DecodeLimits(wrapper.Limits)
	if err != nil {
		return Limits{}
	}
	return l
}

// normalizePlaneLimits validates a plane-wide limits patch. Null clears it.
func normalizePlaneLimits(raw json.RawMessage) (json.RawMessage, error) {
	if isJSONNull(raw) {
		return json.RawMessage(`{}`), nil
	}
	l, err := DecodeLimits(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(l)
}

// RunRequest is what a run asks for on top of the configured limits.
type RunRequest struct {
	// Skill is true for a /skill run, which may use more than the normal default.
	Skill bool
	// SkillMaxToolRounds is the skill's own ceiling from its frontmatter (0 = none).
	SkillMaxToolRounds int
	// Depth is the --depth preset (quick, normal, deep) or empty.
	Depth string
	// Seconds is the --time request in seconds (0 = none).
	Seconds int
}

// ResolvedLimits is the budget one Prompt runs under.
type ResolvedLimits struct {
	ToolRounds int
	Seconds    int
	// Notes lists requests that were clamped, written for the user.
	Notes []string
}

// Wall-clock limit of a run.
func (r ResolvedLimits) Timeout() time.Duration { return time.Duration(r.Seconds) * time.Second }

// depthShare is the part of a skill's round ceiling each --depth preset uses.
var depthShare = map[string]float64{"quick": 0.25, "normal": 0.6, "deep": 1}

// ResolveLimits computes min(plane max, assistant limit, skill ceiling,
// --depth share) for a skill run and the assistant-adjusted default for a
// normal Prompt. The assistant may raise or lower a value but never pass the
// plane maximum.
func ResolveLimits(plane, assistant Limits, run RunRequest) ResolvedLimits {
	planeMax := pick(plane.MaxToolRounds, DefaultMaxToolRounds)
	planeDefault := min(pick(plane.DefaultToolRounds, DefaultToolRounds), planeMax)
	planeSeconds := pick(plane.MaxTurnSeconds, DefaultMaxTurnSeconds)

	assistantMax := min(pick(assistant.MaxToolRounds, planeMax), planeMax)
	assistantDefault := min(pick(assistant.DefaultToolRounds, planeDefault), assistantMax)
	seconds := min(pick(assistant.MaxTurnSeconds, planeSeconds), planeSeconds)

	out := ResolvedLimits{ToolRounds: assistantDefault, Seconds: seconds}
	if !run.Skill {
		return out
	}
	ceiling := assistantMax
	if run.SkillMaxToolRounds > 0 {
		ceiling = min(ceiling, run.SkillMaxToolRounds)
	}
	out.ToolRounds = ceiling
	if share, ok := depthShare[run.Depth]; ok {
		out.ToolRounds = max(1, int(math.Ceil(float64(ceiling)*share)))
	}
	if run.Seconds > 0 {
		if run.Seconds > seconds {
			out.Notes = append(out.Notes, fmt.Sprintf("--time %s is above this assistant's limit; using %s.",
				(time.Duration(run.Seconds)*time.Second).String(), (time.Duration(seconds)*time.Second).String()))
		} else {
			out.Seconds = run.Seconds
		}
	}
	return out
}

func pick(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}
