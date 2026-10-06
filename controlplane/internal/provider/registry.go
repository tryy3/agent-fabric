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
	// APIModes overrides OpenCode wire routing per model id (from model
	// specs); models not listed use the built-in prefix table.
	APIModes map[string]string
}

func NewStreamer(typ, baseURL, apiKey string, opts StreamerOpts) (ChatStreamer, error) {
	client := opts.HTTPClient
	switch typ {
	case catalog.TypeOpenAICompatible:
		return NewOpenAI(baseURL, apiKey, client), nil
	case catalog.TypeUnslothStudio:
		return NewOpenAI(baseURL, apiKey, client).WithUnslothExtras(), nil
	case catalog.TypeBergetAI:
		return NewOpenAI(baseURL, apiKey, client).WithSamplerExtras(), nil
	case catalog.TypeOpenCodeZen, catalog.TypeOpenCodeGo:
		oc, err := NewOpenCode(typ, baseURL, apiKey, opts.SessionID, client)
		if err != nil {
			return nil, err
		}
		return oc.WithAPIModes(opts.APIModes), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", typ)
	}
}
