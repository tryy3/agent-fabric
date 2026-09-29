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
	EnableThinking    *bool
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
}
