package provider

import (
	"fmt"
	"net/http"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func NewStreamer(typ, baseURL, apiKey string, httpClient *http.Client) (ChatStreamer, error) {
	switch typ {
	case catalog.TypeOpenAICompatible:
		return NewOpenAI(baseURL, apiKey, httpClient), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", typ)
	}
}
