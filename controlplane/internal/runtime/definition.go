package runtime

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
	Inference               Inference
	// EffectiveInstructions is Harness + Assistant instructions composed at session/new.
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
