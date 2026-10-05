package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Permissions holds settings.permissions: the permission mode, the user's
// permission rules and the optional gate scorers. On an assistant all three
// apply; the plane-wide default (PlaneSettings.Permissions) carries rules and
// scorers only.
type Permissions struct {
	// Mode is one of the gate permission modes; empty means the default (ask).
	Mode string `json:"mode,omitempty"`
	// Rules always allow, ask about, or deny matching tool calls.
	Rules []PermissionRule `json:"rules,omitempty"`
	// Scorers configures the optional model tiers of the gate cascade.
	Scorers *PermissionScorers `json:"scorers,omitempty"`
	// Builtins overrides built-in rule tiers by ID (GET /v1/permissions/builtins
	// lists them with their defaults).
	Builtins map[string]PermissionBuiltin `json:"builtins,omitempty"`
}

// PermissionBuiltin mirrors gate.TierOverride: a changed base score, whether
// scorers are consulted, and program names added to or removed from a
// program-list tier.
type PermissionBuiltin struct {
	Risk    int      `json:"risk,omitempty"`
	Consult *bool    `json:"consult,omitempty"`
	Add     []string `json:"add,omitempty"`
	Remove  []string `json:"remove,omitempty"`
}

// PermissionRule mirrors gate.UserRule.
type PermissionRule struct {
	Tool   string `json:"tool"`
	Match  string `json:"match"`
	Action string `json:"action"`
	Risk   int    `json:"risk,omitempty"`
}

// PermissionScorers configures the gate cascade's model tiers. Both are
// optional: with neither, the gate is rules only.
type PermissionScorers struct {
	// Fast is a System One decision model (Jev is the recommended one).
	Fast *PermissionScorer `json:"fast,omitempty"`
	// Deep is a chat model, asked when Fast is absent, unsure or far below the rules.
	Deep *PermissionScorer `json:"deep,omitempty"`
	// MinConfidence is the Fast confidence below which Deep is asked (default 0.6).
	MinConfidence float64 `json:"minConfidence,omitempty"`
	// MaxLower caps how many points Deep may lower a score (default 2, 0 = never).
	MaxLower *int `json:"maxLower,omitempty"`
	// SkipAtOrBelow settles calls the rules score this low without a scorer (default 2).
	SkipAtOrBelow *int `json:"skipAtOrBelow,omitempty"`
}

// PermissionScorer names the inference connection and model of one tier.
type PermissionScorer struct {
	ConnectionID string `json:"connectionId"`
	Model        string `json:"model"`
	// Strategy is the System One answer strategy (fast tier only; default score).
	Strategy string `json:"strategy,omitempty"`
	// The rest apply to the deep tier only.
	// Style is the prompt style: bands (default) or score.
	Style string `json:"style,omitempty"`
	// EnableThinking and ReasoningEffort are sent when set. Scoring needs no
	// reasoning pass: thinking defaults to off where the provider has the switch.
	EnableThinking  *bool  `json:"enableThinking,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// MaxTokens caps the reply, reasoning included (default 1024).
	MaxTokens int `json:"maxTokens,omitempty"`
	// StructuredOutput constrains the reply to the answer's JSON schema on
	// providers that support response_format json_schema.
	StructuredOutput bool `json:"structuredOutput,omitempty"`
}

// PermissionsFromSettings extracts settings.permissions from an agent settings blob.
func PermissionsFromSettings(settings json.RawMessage) (Permissions, error) {
	if len(settings) == 0 || bytes.Equal(bytes.TrimSpace(settings), []byte("null")) {
		return Permissions{}, nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(settings, &bag); err != nil {
		return Permissions{}, fmt.Errorf("decode settings: %w", err)
	}
	raw, ok := bag["permissions"]
	if !ok || isJSONNull(raw) {
		return Permissions{}, nil
	}
	return DecodePermissions(raw)
}

// DecodePermissions validates and decodes a permissions JSON object.
func DecodePermissions(raw json.RawMessage) (Permissions, error) {
	if !isJSONObject(raw) {
		return Permissions{}, fmt.Errorf("permissions must be an object")
	}
	var p Permissions
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Permissions{}, fmt.Errorf("permissions: %s", strings.TrimPrefix(err.Error(), "json: "))
	}
	if p.Mode != "" && !slices.Contains(PermissionModes, p.Mode) {
		return Permissions{}, fmt.Errorf("permissions.mode must be one of %s", strings.Join(PermissionModes, ", "))
	}
	for i, r := range p.Rules {
		if err := r.validate(); err != nil {
			return Permissions{}, fmt.Errorf("permissions.rules[%d]: %w", i, err)
		}
	}
	if p.Scorers != nil {
		if err := p.Scorers.validate(); err != nil {
			return Permissions{}, fmt.Errorf("permissions.scorers: %w", err)
		}
	}
	if len(p.Builtins) > 0 {
		for id, b := range p.Builtins {
			if b.Risk != 0 && (b.Risk < 1 || b.Risk > 10) {
				return Permissions{}, fmt.Errorf("permissions.builtins.%s: risk must be 1-10", id)
			}
		}
		if ValidatePermissionBuiltinsFunc != nil {
			if err := ValidatePermissionBuiltinsFunc(p.Builtins); err != nil {
				return Permissions{}, fmt.Errorf("permissions.builtins: %w", err)
			}
		}
	}
	return p, nil
}

func (r PermissionRule) validate() error {
	switch {
	case strings.TrimSpace(r.Tool) == "":
		return fmt.Errorf("tool is required")
	case strings.TrimSpace(r.Match) == "":
		return fmt.Errorf("match is required")
	case !slices.Contains(PermissionRuleActions, r.Action):
		return fmt.Errorf("action must be one of %s", strings.Join(PermissionRuleActions, ", "))
	case r.Risk != 0 && (r.Risk < 1 || r.Risk > 10):
		return fmt.Errorf("risk must be 1-10")
	}
	return nil
}

func (s PermissionScorers) validate() error {
	for name, sc := range map[string]*PermissionScorer{"fast": s.Fast, "deep": s.Deep} {
		if sc == nil {
			continue
		}
		if strings.TrimSpace(sc.ConnectionID) == "" || strings.TrimSpace(sc.Model) == "" {
			return fmt.Errorf("%s needs connectionId and model", name)
		}
	}
	if s.Fast != nil && s.Fast.Strategy != "" && !slices.Contains(PermissionScorerStrategies, s.Fast.Strategy) {
		return fmt.Errorf("fast.strategy must be one of %s", strings.Join(PermissionScorerStrategies, ", "))
	}
	if s.Deep != nil && s.Deep.Strategy != "" {
		return fmt.Errorf("deep.strategy is not supported")
	}
	if d := s.Deep; d != nil {
		if d.Style != "" && !slices.Contains(PermissionScorerStyles, d.Style) {
			return fmt.Errorf("deep.style must be one of %s", strings.Join(PermissionScorerStyles, ", "))
		}
		if d.MaxTokens < 0 || d.MaxTokens > 16384 {
			return fmt.Errorf("deep.maxTokens must be 0-16384")
		}
	}
	if f := s.Fast; f != nil && (f.Style != "" || f.EnableThinking != nil || f.ReasoningEffort != "" || f.MaxTokens != 0 || f.StructuredOutput) {
		return fmt.Errorf("fast supports connectionId, model and strategy only")
	}
	if s.MinConfidence < 0 || s.MinConfidence > 1 {
		return fmt.Errorf("minConfidence must be between 0 and 1")
	}
	if s.MaxLower != nil && (*s.MaxLower < 0 || *s.MaxLower > 9) {
		return fmt.Errorf("maxLower must be 0-9")
	}
	if s.SkipAtOrBelow != nil && (*s.SkipAtOrBelow < 0 || *s.SkipAtOrBelow > 8) {
		return fmt.Errorf("skipAtOrBelow must be 0-8")
	}
	return nil
}

// ValidatePermissionBuiltinsFunc checks built-in tier overrides against the
// gate's tiers and PermissionBuiltinTiersFunc lists those tiers. The server
// sets both: the catalog cannot import the gate.
var (
	ValidatePermissionBuiltinsFunc func(map[string]PermissionBuiltin) error
	PermissionBuiltinTiersFunc     func() any
)

// EffectivePermissions combines the plane-wide default with an assistant's
// permissions: plane rules first, then the assistant's (the most restrictive
// matching rule wins either way); the assistant's scorers replace the plane's
// when set; built-in overrides apply per tier, the assistant's winning; the
// mode is the assistant's.
func EffectivePermissions(plane, assistant Permissions) Permissions {
	out := Permissions{Mode: assistant.Mode, Scorers: plane.Scorers}
	out.Rules = append(append(out.Rules, plane.Rules...), assistant.Rules...)
	if len(plane.Builtins)+len(assistant.Builtins) > 0 {
		out.Builtins = map[string]PermissionBuiltin{}
		for id, b := range plane.Builtins {
			out.Builtins[id] = b
		}
		for id, b := range assistant.Builtins {
			out.Builtins[id] = b
		}
	}
	if assistant.Scorers != nil {
		out.Scorers = assistant.Scorers
	}
	return out
}

// The lists below mirror the gate package (gate tests keep them in sync); the
// catalog cannot import the gate without a cycle through the provider package.

// PermissionModes lists the valid settings.permissions.mode values, least to
// most permissive.
var PermissionModes = []string{"ask", "auto_approve", "auto", "full"}

// PermissionRuleActions lists the valid rule actions, most to least restrictive.
var PermissionRuleActions = []string{"deny", "ask", "allow"}

// PermissionScorerStyles lists the valid deep-tier prompt styles.
var PermissionScorerStyles = []string{"score", "bands"}

// PermissionScorerStrategies lists the valid fast-tier strategies.
var PermissionScorerStrategies = []string{"score", "questions", "bands"}
