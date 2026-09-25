package provider

import (
	"fmt"
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// StreamerOpts configures NewStreamer.
type StreamerOpts struct {
	SessionID  string
	HTTPClient *http.Client
}

func NewStreamer(typ, baseURL, apiKey string, opts StreamerOpts) (ChatStreamer, error) {
	client := opts.HTTPClient
	switch typ {
	case catalog.TypeOpenAICompatible:
		return NewOpenAI(baseURL, apiKey, client), nil
	case catalog.TypeOpenCodeZen, catalog.TypeOpenCodeGo:
		return NewOpenCode(typ, baseURL, apiKey, opts.SessionID, client)
	default:
		return nil, fmt.Errorf("unknown provider type %q", typ)
	}
}
