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
	mode := OpenCodeModelAPIMode(o.providerType, model)
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
