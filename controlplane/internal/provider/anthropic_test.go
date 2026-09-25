package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestAnthropicStreamsTextAndHeaders(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("User-Agent") != "agent-fabric/1.0" {
			t.Fatalf("ua = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("x-opencode-session") != "sess_abc" {
			t.Fatalf("session = %q", r.Header.Get("x-opencode-session"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client := provider.NewAnthropic(srv.URL+"/v1", "sk-test", srv.Client()).WithExtraHeaders(map[string]string{
		"User-Agent":          "agent-fabric/1.0",
		"x-opencode-session": "sess_abc",
	})
	var parts []string
	err := client.StreamChat(context.Background(), "claude-sonnet-5", []runtime.Message{
		{Role: "user", Content: "hi"},
	}, provider.StreamChatOptions{}, func(ev provider.StreamEvent) error {
		if ev.Content != "" {
			parts = append(parts, ev.Content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if strings.Join(parts, "") != "Hi" {
		t.Fatalf("parts = %#v", parts)
	}
	if gotBody["model"] != "claude-sonnet-5" {
		t.Fatalf("model = %v", gotBody["model"])
	}
}

func TestAnthropicStreamsToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `data: {"type":"content_block_start","content_block":{"type":"tool_use","id":"tu_1","name":"read_file"}}`+"\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, `data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.txt\"}"}}`+"\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`+"\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client := provider.NewAnthropic(srv.URL+"/v1", "sk", srv.Client())
	var calls []provider.ToolCall
	err := client.StreamChat(context.Background(), "claude-sonnet-5", []runtime.Message{
		{Role: "user", Content: "read"},
	}, provider.StreamChatOptions{}, func(ev provider.StreamEvent) error {
		if ev.Finish == "tool_calls" {
			calls = ev.ToolCalls
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "tu_1" || calls[0].Name != "read_file" {
		t.Fatalf("calls = %#v", calls)
	}
	if !strings.Contains(calls[0].Arguments, "a.txt") {
		t.Fatalf("args = %q", calls[0].Arguments)
	}
}
