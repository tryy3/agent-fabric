package gatebench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
)

// Config is the benchmark setup file: a list of named gate setups to compare.
type Config struct {
	Setups []SetupConfig `json:"setups"`
}

// SetupConfig is one gate: an ordered evaluator list run as a gate.Chain, or
// as a gate.Cascade when Cascade is set.
type SetupConfig struct {
	Name       string            `json:"name"`
	Evaluators []EvaluatorConfig `json:"evaluators"`
	Cascade    *CascadeConfig    `json:"cascade,omitempty"`
}

// CascadeConfig runs the evaluators as tiers instead of taking the highest
// score: rules first, then an optional systemone evaluator (fast tier), then
// an optional llm evaluator (deep tier), which is only asked when the fast
// tier is unsure and may lower the score.
type CascadeConfig struct {
	// MinConfidence is the fast tier confidence below which the deep tier is
	// asked (default gate.DefaultMinConfidence).
	MinConfidence float64 `json:"minConfidence,omitempty"`
	// SkipAtOrBelow settles calls the rules score this low without a scorer
	// (default 0: always consult).
	SkipAtOrBelow int `json:"skipAtOrBelow,omitempty"`
	// MaxLower caps how many points the deep tier may lower the score
	// (default gate.DefaultMaxLower; 0 = never lower).
	MaxLower *int `json:"maxLower,omitempty"`
}

// EvaluatorConfig is one slot in a setup. Type "rules" is the deterministic
// gate.Rules; type "llm" is a gate.LLMScorer against any chat model; type
// "systemone" is a gate.SystemOneScorer against a System One decision model
// (hosted Jev, or a Laya server) through POST /v1/systemone. Tools
// limits an evaluator to those tool names (it abstains on the rest), so
// different models can gate different kinds of calls.
type EvaluatorConfig struct {
	Type  string   `json:"type"`
	Name  string   `json:"name,omitempty"`
	Tools []string `json:"tools,omitempty"`

	Connection     *ConnectionConfig `json:"connection,omitempty"`
	Model          string            `json:"model,omitempty"`
	FailRisk       int               `json:"failRisk,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
	// StateFormat (systemone only): "object" (default, {"body": ...}) or "text"
	// (a plain string, as in the Jev, Berget and OpenCode examples).
	StateFormat string `json:"stateFormat,omitempty"`
	// Strategy (systemone only): "questions" (default, several yes/no questions),
	// "score" (one ten-level score question) or "bands" (one five-level band
	// question plus whether the user requested the call).
	Strategy string `json:"strategy,omitempty"`
	// Style (llm only): "score" (default, a 1-10 score) or "bands" (a risk band
	// plus whether the user requested the call).
	Style string `json:"style,omitempty"`
	// MaxTokens caps the reply including reasoning (default 1024). Thinking
	// models need room, or a lower ReasoningEffort / EnableThinking false.
	MaxTokens       int     `json:"maxTokens,omitempty"`
	ReasoningEffort *string `json:"reasoningEffort,omitempty"`
	EnableThinking  *bool   `json:"enableThinking,omitempty"`
}

// ConnectionConfig names an inference endpoint: Type (default
// openai_compatible) and BaseURL, which built-in types may omit. Prefer
// APIKeyEnv over APIKey so keys stay out of files.
type ConnectionConfig struct {
	Type      string `json:"type,omitempty"`
	BaseURL   string `json:"baseUrl,omitempty"`
	APIKey    string `json:"apiKey,omitempty"`
	APIKeyEnv string `json:"apiKeyEnv,omitempty"`
}

// LoadConfig decodes a setup file strictly.
func LoadConfig(raw []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode setups: %w", err)
	}
	if len(cfg.Setups) == 0 {
		return Config{}, fmt.Errorf("no setups defined")
	}
	seen := map[string]bool{}
	for _, s := range cfg.Setups {
		if strings.TrimSpace(s.Name) == "" || seen[s.Name] {
			return Config{}, fmt.Errorf("setup name %q is empty or duplicated", s.Name)
		}
		seen[s.Name] = true
		if len(s.Evaluators) == 0 {
			return Config{}, fmt.Errorf("setup %q has no evaluators", s.Name)
		}
		if s.Cascade != nil {
			if err := checkCascade(s); err != nil {
				return Config{}, err
			}
		}
		for _, e := range s.Evaluators {
			switch e.Type {
			case "rules":
			case "systemone":
				if e.StateFormat != "" && e.StateFormat != gate.StateObject && e.StateFormat != gate.StateText {
					return Config{}, fmt.Errorf("setup %q: systemone stateFormat must be %q or %q", s.Name, gate.StateObject, gate.StateText)
				}
				if e.Strategy != "" && e.Strategy != gate.StrategyQuestions && e.Strategy != gate.StrategyScore && e.Strategy != gate.StrategyBands {
					return Config{}, fmt.Errorf("setup %q: systemone strategy must be %q, %q or %q", s.Name, gate.StrategyQuestions, gate.StrategyScore, gate.StrategyBands)
				}
				if e.Model == "" && (e.Connection == nil || e.Connection.BaseURL == "") {
					return Config{}, fmt.Errorf("setup %q: systemone evaluator needs a model (hosted Jev) or connection.baseUrl (e.g. a local Laya server)", s.Name)
				}
			case "llm":
				if e.Model == "" || e.Connection == nil {
					return Config{}, fmt.Errorf("setup %q: llm evaluator needs connection and model", s.Name)
				}
				if e.Style != "" && e.Style != gate.StyleScore && e.Style != gate.StyleBands {
					return Config{}, fmt.Errorf("setup %q: llm style must be %q or %q", s.Name, gate.StyleScore, gate.StyleBands)
				}
			default:
				return Config{}, fmt.Errorf("setup %q: unknown evaluator type %q", s.Name, e.Type)
			}
		}
	}
	return cfg, nil
}

func scorerLabel(ec EvaluatorConfig) string {
	if ec.Name != "" {
		return ec.Name
	}
	return ec.Type
}

// checkCascade requires the tier order rules, [systemone], [llm].
func checkCascade(s SetupConfig) error {
	var order []string
	for _, e := range s.Evaluators {
		order = append(order, e.Type)
	}
	switch strings.Join(order, ",") {
	case "rules,systemone,llm", "rules,systemone", "rules,llm":
		return nil
	}
	return fmt.Errorf("setup %q: a cascade needs evaluators rules, then systemone and/or llm, in that order", s.Name)
}

// StreamerFactory builds the chat client for a resolved connection. Tests
// substitute a scripted one; the default is provider.NewStreamer.
type StreamerFactory func(typ, baseURL, apiKey string) (provider.ChatStreamer, error)

// BuildOptions injects dependencies of Build.
type BuildOptions struct {
	Streamer StreamerFactory // optional: defaults to provider.NewStreamer
}

// Setup is a built gate ready to run.
type Setup struct {
	Name string
	Gate gate.Evaluator
	// Scorers are the LLM slots' counters, for usage reporting; ScorerNames
	// labels them (evaluator name, else type).
	Scorers     []*gate.ScorerStats
	ScorerNames []string
}

// Build constructs the chain of a setup.
func Build(ctx context.Context, sc SetupConfig, opts BuildOptions) (*Setup, error) {
	newStreamer := opts.Streamer
	if newStreamer == nil {
		newStreamer = func(typ, baseURL, apiKey string) (provider.ChatStreamer, error) {
			return provider.NewStreamer(typ, baseURL, apiKey, provider.StreamerOpts{})
		}
	}
	s := &Setup{Name: sc.Name}
	var evaluators []gate.Evaluator
	cascade := gate.Cascade{}
	for i, ec := range sc.Evaluators {
		var ev gate.Evaluator
		switch ec.Type {
		case "rules":
			ev = gate.Rules{}
		case "systemone":
			var c ConnectionConfig
			if ec.Connection != nil {
				c = *ec.Connection
			}
			key := c.APIKey
			if c.APIKeyEnv != "" {
				if key = os.Getenv(c.APIKeyEnv); key == "" {
					return nil, fmt.Errorf("setup %q evaluator %d: environment variable %s is empty", sc.Name, i, c.APIKeyEnv)
				}
			}
			stats := &gate.ScorerStats{}
			s.Scorers = append(s.Scorers, stats)
			s.ScorerNames = append(s.ScorerNames, scorerLabel(ec))
			ev = gate.SystemOneScorer{
				BaseURL: c.BaseURL, APIKey: key, Model: ec.Model, Name: ec.Name, Strategy: ec.Strategy, StateFormat: ec.StateFormat,
				FailRisk: ec.FailRisk, Timeout: time.Duration(ec.TimeoutSeconds) * time.Second,
				Stats: stats,
			}
		case "llm":
			typ, baseURL, apiKey, err := resolveConnection(*ec.Connection)
			if err != nil {
				return nil, fmt.Errorf("setup %q evaluator %d: %w", sc.Name, i, err)
			}
			st, err := newStreamer(typ, baseURL, apiKey)
			if err != nil {
				return nil, fmt.Errorf("setup %q evaluator %d: %w", sc.Name, i, err)
			}
			stats := &gate.ScorerStats{}
			s.Scorers = append(s.Scorers, stats)
			s.ScorerNames = append(s.ScorerNames, scorerLabel(ec))
			ev = gate.LLMScorer{
				Streamer: st, Model: ec.Model, Name: ec.Name, Style: ec.Style,
				FailRisk:  ec.FailRisk,
				MaxTokens: ec.MaxTokens, ReasoningEffort: ec.ReasoningEffort, EnableThinking: ec.EnableThinking,
				Timeout: time.Duration(ec.TimeoutSeconds) * time.Second,
				Stats:   stats,
			}
		}
		if len(ec.Tools) > 0 {
			ev = toolFilter{inner: ev, tools: ec.Tools}
		}
		evaluators = append(evaluators, ev)
		switch ec.Type {
		case "rules":
			cascade.Rules = ev
		case "systemone":
			cascade.Fast = ev
		case "llm":
			cascade.Deep = ev
		}
	}
	if sc.Cascade == nil {
		s.Gate = gate.Chain{Evaluators: evaluators}
		return s, nil
	}
	cascade.MinConfidence, cascade.SkipAtOrBelow = sc.Cascade.MinConfidence, sc.Cascade.SkipAtOrBelow
	if ml := sc.Cascade.MaxLower; ml != nil {
		if cascade.MaxLower = *ml; *ml <= 0 {
			cascade.MaxLower = -1
		}
	}
	s.Gate = cascade
	return s, nil
}

func resolveConnection(c ConnectionConfig) (typ, baseURL, key string, err error) {
	typ, baseURL, key = c.Type, c.BaseURL, c.APIKey
	if typ == "" {
		typ = catalog.TypeOpenAICompatible
	}
	if baseURL == "" {
		baseURL = catalog.FixedBaseURL(typ)
	}
	if c.APIKeyEnv != "" {
		key = os.Getenv(c.APIKeyEnv)
		if key == "" {
			return "", "", "", fmt.Errorf("environment variable %s is empty", c.APIKeyEnv)
		}
	}
	if baseURL == "" {
		return "", "", "", fmt.Errorf("connection needs baseUrl")
	}
	return typ, baseURL, key, nil
}

// toolFilter limits an evaluator to some tools; other calls get an unscored Allow.
type toolFilter struct {
	inner gate.Evaluator
	tools []string
}

func (f toolFilter) Evaluate(ctx context.Context, req gate.Request) (gate.Decision, error) {
	for _, t := range f.tools {
		if t == req.ToolName {
			return f.inner.Evaluate(ctx, req)
		}
	}
	return gate.Decision{Kind: gate.Allow}, nil
}

// RulesSetup is the built-in deterministic setup.
func RulesSetup() *Setup {
	return &Setup{Name: "rules", Gate: gate.DefaultChain()}
}
