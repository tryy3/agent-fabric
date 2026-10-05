package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func sseServer(t *testing.T, lines ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			_, _ = io.WriteString(w, "data: "+l+"\n\n")
		}
	}))
}

func collectUsage(t *testing.T, s provider.ChatStreamer) *provider.Usage {
	t.Helper()
	var usage *provider.Usage
	err := s.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "hi"}},
		provider.StreamChatOptions{}, func(ev provider.StreamEvent) error {
			if ev.Usage != nil {
				usage = ev.Usage
			}
			return nil
		})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if usage == nil {
		t.Fatal("no usage event")
	}
	return usage
}

func intIs(p *int, want int) bool { return p != nil && *p == want }

func TestOpenAIParsesCachedReasoningAndReportedCost(t *testing.T) {
	srv := sseServer(t,
		`{"choices":[{"delta":{"content":"ok"}}]}`,
		`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140,`+
			`"prompt_tokens_details":{"cached_tokens":60},"completion_tokens_details":{"reasoning_tokens":25},"cost":0.0123}}`,
		`[DONE]`)
	defer srv.Close()
	u := collectUsage(t, provider.NewOpenAI(srv.URL, "k", srv.Client()))
	if !intIs(u.CachedTokens, 60) || !intIs(u.ReasoningTokens, 25) || !intIs(u.PromptTokens, 100) {
		t.Fatalf("usage = %+v", u)
	}
	if u.ReportedCostUSD == nil || *u.ReportedCostUSD != 0.0123 {
		t.Fatalf("reported cost = %v", u.ReportedCostUSD)
	}
	if _, ok := u.Extras["cost"]; ok {
		t.Fatalf("numeric cost must not stay in extras: %v", u.Extras)
	}
}

func TestOpenAIToleratesNonNumericCostAndDetails(t *testing.T) {
	srv := sseServer(t,
		`{"choices":[{"delta":{"content":"ok"}}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,`+
			`"cost":{"upstream":1},"prompt_tokens_details":null}}`,
		`[DONE]`)
	defer srv.Close()
	u := collectUsage(t, provider.NewOpenAI(srv.URL, "k", srv.Client()))
	if u.ReportedCostUSD != nil || u.CachedTokens != nil {
		t.Fatalf("usage = %+v", u)
	}
	if _, ok := u.Extras["cost"]; !ok {
		t.Fatalf("non-numeric cost should stay visible in extras: %v", u.Extras)
	}
}

func TestAnthropicPromptTokensIncludeCache(t *testing.T) {
	srv := sseServer(t,
		`{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":200,"cache_creation_input_tokens":30,"output_tokens":1}}}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hi"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`)
	defer srv.Close()
	u := collectUsage(t, provider.NewAnthropic(srv.URL+"/v1", "k", srv.Client()))
	if !intIs(u.PromptTokens, 240) || !intIs(u.CachedTokens, 200) || !intIs(u.CacheWriteTokens, 30) {
		t.Fatalf("usage = %+v", u)
	}
	if !intIs(u.CompletionTokens, 7) || !intIs(u.TotalTokens, 247) {
		t.Fatalf("totals = %+v", u)
	}
}

func TestResponsesParsesCachedAndReasoning(t *testing.T) {
	srv := sseServer(t,
		`{"type":"response.output_text.delta","delta":"Hi"}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":50,"output_tokens":20,"total_tokens":70,`+
			`"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":12}}}}`)
	defer srv.Close()
	u := collectUsage(t, provider.NewResponses(srv.URL+"/v1", "k", srv.Client()))
	if !intIs(u.CachedTokens, 30) || !intIs(u.ReasoningTokens, 12) || !intIs(u.PromptTokens, 50) {
		t.Fatalf("usage = %+v", u)
	}
}
