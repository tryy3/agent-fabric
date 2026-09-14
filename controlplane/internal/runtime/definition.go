package runtime

type ModelRef struct {
	ID   string
	Name string
}

type SessionPin struct {
	AgentID      string
	AgentName    string
	AgentVersion int
	ProviderID   string
	ProviderName string
	ProviderType string
	BaseURL      string
	APIKey       string
	Models       []ModelRef
	CurrentModel string
}
