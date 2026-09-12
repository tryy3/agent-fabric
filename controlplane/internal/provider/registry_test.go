package provider_test

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestNewStreamerOpenAICompatible(t *testing.T) {
	s, err := provider.NewStreamer(catalog.TypeOpenAICompatible, "http://example/v1", "sk", nil)
	if err != nil {
		t.Fatalf("NewStreamer: %v", err)
	}
	if _, ok := s.(*provider.OpenAI); !ok {
		t.Fatalf("got %T, want *provider.OpenAI", s)
	}
}

func TestNewStreamerUnknownType(t *testing.T) {
	_, err := provider.NewStreamer("unknown_type", "http://example/v1", "sk", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown_type") {
		t.Fatalf("err = %v, want unknown type", err)
	}
}
