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

// SetupConfig is one gate: an ordered evaluator list run as a gate.Chain.
type SetupConfig struct {
	Name       string            `json:"name"`
	Evaluators []EvaluatorConfig `json:"evaluators"`
}

// EvaluatorConfig is one slot in a setup. Type "rules" is the deterministic
// gate.Rules; type "llm" is a gate.LLMScorer against any chat model. Tools
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
		for _, e := range s.Evaluators {
			switch e.Type {
			case "rules":
			case "llm":
				if e.Model == "" || e.Connection == nil {
					return Config{}, fmt.Errorf("setup %q: llm evaluator needs connection and model", s.Name)
				}
			default:
				return Config{}, fmt.Errorf("setup %q: unknown evaluator type %q", s.Name, e.Type)
			}
		}
	}
	return cfg, nil
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
	Name  string
	Chain gate.Chain
	// Scorers are the LLM slots' counters, for usage reporting.
	Scorers []*gate.ScorerStats
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
	for i, ec := range sc.Evaluators {
		var ev gate.Evaluator
		switch ec.Type {
		case "rules":
			ev = gate.Rules{}
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
			ev = gate.LLMScorer{
				Streamer: st, Model: ec.Model, Name: ec.Name,
				FailRisk:  ec.FailRisk,
				MaxTokens: ec.MaxTokens, ReasoningEffort: ec.ReasoningEffort, EnableThinking: ec.EnableThinking,
				Timeout: time.Duration(ec.TimeoutSeconds) * time.Second,
				Stats:   stats,
			}
		}
		if len(ec.Tools) > 0 {
			ev = toolFilter{inner: ev, tools: ec.Tools}
		}
		s.Chain.Evaluators = append(s.Chain.Evaluators, ev)
	}
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
	return &Setup{Name: "rules", Chain: gate.DefaultChain()}
}
