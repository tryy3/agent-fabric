package runtime

import "github.com/tryy3/agent-fabric/internal/modelspecs"

type ModelRef struct {
	ID   string
	Name string
}

// Inference is a snapshot of agent settings.inference at session/new.
type Inference struct {
	Temperature       *float64
	TopP              *float64
	MaxTokens         *int
	ReasoningEffort   *string
	TopK              *int
	MinP              *float64
	RepetitionPenalty *float64
	PresencePenalty   *float64
	FrequencyPenalty  *float64
	EnableThinking    *bool
	ThinkingType      *string
}

type SessionPin struct {
	AssistantID             string
	AssistantName           string
	AssistantVersion        int
	InferenceConnectionID   string
	InferenceConnectionName string
	ConnectionType          string
	BaseURL                 string
	APIKey                  string
	Models                  []ModelRef
	CurrentModel            string
	// Prices are per-million-token prices of the connection's models pinned at
	// session/new (keyed by model id); used for cost estimates only. Nil or a
	// missing key means no estimate.
	Prices    map[string]*modelspecs.Cost
	Inference Inference
	// PermissionMode is the gate permission mode pinned at session/new (empty = ask).
	PermissionMode string
	// Gate is the rest of the gate configuration pinned at session/new.
	Gate GatePin
	// EffectiveInstructions is Platform + Assistant + Runtime Context instructions
	// composed and variable-substituted at session/new.
	EffectiveInstructions string
	// Web tool integrations pinned at session/new (nil = capability disabled).
	WebSearch *WebIntegrationPin
	FetchPage *WebIntegrationPin
}

// WebIntegrationPin is a session snapshot of one tool integration.
type WebIntegrationPin struct {
	ID       string
	Name     string
	Kind     string
	Endpoint string
	Mode     string
	Secrets  map[string]string
	Config   []byte
}

// GatePin is the tool gate configuration of a session: the effective
// permission rules (plane-wide, then the assistant's) and the optional scorer
// tiers with their connections resolved.
type GatePin struct {
	Rules []PermissionRule
	// Builtins overrides built-in rule tiers by ID.
	Builtins map[string]BuiltinOverride
	// Fast is the System One tier, Deep the chat model tier; nil when not configured.
	Fast, Deep *GateScorer
	// MinConfidence, MaxLower and SkipAtOrBelow tune the cascade; zero and nil
	// mean the defaults.
	MinConfidence float64
	MaxLower      *int
	SkipAtOrBelow *int
}

// PermissionRule always allows, asks about, or denies matching tool calls.
type PermissionRule struct {
	Tool, Match, Action string
	Risk                int
	Consult             bool
}

// GateScorer is one scorer tier with its inference connection resolved.
type GateScorer struct {
	ConnectionType, BaseURL, APIKey string
	Model                           string
	// Strategy is the System One answer strategy (fast tier only).
	Strategy string
	// Deep tier only: prompt style, thinking switches, reply cap and whether
	// the reply is constrained to a JSON schema.
	Style            string
	EnableThinking   *bool
	ReasoningEffort  string
	MaxTokens        int
	StructuredOutput bool
}

// BuiltinOverride changes one built-in rule tier.
type BuiltinOverride struct {
	Risk        int
	Consult     *bool
	Add, Remove []string
}
