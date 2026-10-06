package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

const openCodeUserAgent = "agent-fabric/1.0"

// OpenCode routes each model to the correct OpenCode wire API.
type OpenCode struct {
	providerType string
	baseURL      string
	apiKey       string
	sessionID    string
	httpClient   *http.Client
	chat         *OpenAI
	anthropic    *Anthropic
	responses    *Responses
	apiModes     map[string]string
}

// WithAPIModes sets per-model wire API overrides (from model specs). Unknown
// mode values are ignored so a bad specs document cannot break routing.
func (o *OpenCode) WithAPIModes(modes map[string]string) *OpenCode {
	o.apiModes = make(map[string]string, len(modes))
	for model, mode := range modes {
		switch mode {
		case APIModeChatCompletions, APIModeAnthropicMessages, APIModeCodexResponses:
			o.apiModes[model] = mode
		}
	}
	return o
}

// apiMode is the specs-provided wire API for a model, else the built-in routing.
func (o *OpenCode) apiMode(model string) string {
	if mode, ok := o.apiModes[model]; ok {
		return mode
	}
	return OpenCodeModelAPIMode(o.providerType, model)
}

func NewOpenCode(providerType, baseURL, apiKey, sessionID string, httpClient *http.Client) (*OpenCode, error) {
	if !catalog.IsOpenCodeType(providerType) {
		return nil, fmt.Errorf("not an OpenCode provider type %q", providerType)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	headers := openCodeHeaders(sessionID)
	return &OpenCode{
		providerType: providerType,
		baseURL:      baseURL,
		apiKey:       apiKey,
		sessionID:    sessionID,
		httpClient:   httpClient,
		chat:         NewOpenAI(baseURL, apiKey, httpClient).WithExtraHeaders(headers),
		anthropic:    NewAnthropic(baseURL, apiKey, httpClient).WithExtraHeaders(headers),
		responses:    NewResponses(baseURL, apiKey, httpClient).WithExtraHeaders(headers),
	}, nil
}

func openCodeHeaders(sessionID string) map[string]string {
	h := map[string]string{
		"User-Agent": openCodeUserAgent,
	}
	if sessionID != "" {
		h["x-opencode-session"] = sessionID
	}
	return h
}

func (o *OpenCode) StreamChat(ctx context.Context, model string, messages []runtime.Message, opts StreamChatOptions, onEvent func(StreamEvent) error) error {
	mode := o.apiMode(model)
	switch mode {
	case APIModeAnthropicMessages:
		return o.anthropic.StreamChat(ctx, model, messages, opts, onEvent)
	case APIModeCodexResponses:
		return o.responses.StreamChat(ctx, model, messages, opts, onEvent)
	case APIModeChatCompletions:
		return o.chat.StreamChat(ctx, model, messages, opts, onEvent)
	default:
		return fmt.Errorf("unsupported OpenCode api mode %q for model %q", mode, model)
	}
}

var _ ChatStreamer = (*OpenCode)(nil)
