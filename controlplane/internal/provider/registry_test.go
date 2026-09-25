package provider_test

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestNewStreamerOpenAICompatible(t *testing.T) {
	s, err := provider.NewStreamer(catalog.TypeOpenAICompatible, "http://example/v1", "sk", provider.StreamerOpts{})
	if err != nil {
		t.Fatalf("NewStreamer: %v", err)
	}
	if _, ok := s.(*provider.OpenAI); !ok {
		t.Fatalf("got %T, want *provider.OpenAI", s)
	}
}

func TestNewStreamerOpenCode(t *testing.T) {
	for _, typ := range []string{catalog.TypeOpenCodeZen, catalog.TypeOpenCodeGo} {
		s, err := provider.NewStreamer(typ, catalog.FixedBaseURL(typ), "sk", provider.StreamerOpts{SessionID: "sess_1"})
		if err != nil {
			t.Fatalf("NewStreamer(%s): %v", typ, err)
		}
		if _, ok := s.(*provider.OpenCode); !ok {
			t.Fatalf("got %T, want *provider.OpenCode", s)
		}
	}
}

func TestNewStreamerUnknownType(t *testing.T) {
	_, err := provider.NewStreamer("unknown_type", "http://example/v1", "sk", provider.StreamerOpts{})
	if err == nil || !strings.Contains(err.Error(), "unknown_type") {
		t.Fatalf("err = %v, want unknown type", err)
	}
}
