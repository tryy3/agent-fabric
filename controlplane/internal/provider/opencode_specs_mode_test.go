package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestWireModeConstantsMatchSpecs(t *testing.T) {
	if provider.APIModeChatCompletions != modelspecs.WireChatCompletions ||
		provider.APIModeAnthropicMessages != modelspecs.WireAnthropicMessages ||
		provider.APIModeCodexResponses != modelspecs.WireCodexResponses {
		t.Fatal("provider API modes and modelspecs wire modes diverged")
	}
}

func TestOpenCodeSpecsModesOverrideBuiltInRouting(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"a\"}}\n\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"c\"}}]}\n\ndata: [DONE]\n\n")
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	s, err := provider.NewStreamer(catalog.TypeOpenCodeZen, srv.URL+"/v1", "sk", provider.StreamerOpts{
		HTTPClient: srv.Client(),
		APIModes: map[string]string{
			"brand-new-model": modelspecs.WireAnthropicMessages, // unknown prefix: table says chat
			"claude-sonnet-5": modelspecs.WireChatCompletions,   // table says anthropic; specs win
			"bogus":           "not-a-mode",                     // ignored
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	run := func(model string) {
		t.Helper()
		err := s.StreamChat(context.Background(), model, []runtime.Message{{Role: "user", Content: "x"}},
			provider.StreamChatOptions{}, func(provider.StreamEvent) error { return nil })
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
	}
	for model, want := range map[string]string{
		"brand-new-model": "/v1/messages",
		"claude-sonnet-5": "/v1/chat/completions",
		"bogus":           "/v1/chat/completions", // invalid specs mode falls back to the table (chat)
		"unlisted-model":  "/v1/chat/completions",
	} {
		run(model)
		if path != want {
			t.Errorf("%s routed to %s, want %s", model, path, want)
		}
	}
}
