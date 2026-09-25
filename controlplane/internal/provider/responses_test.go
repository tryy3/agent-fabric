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

func TestResponsesStreamsText(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Yo\"}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client := provider.NewResponses(srv.URL+"/v1", "sk", srv.Client())
	var parts []string
	err := client.StreamChat(context.Background(), "gpt-5.5", []runtime.Message{
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
	if strings.Join(parts, "") != "Yo" {
		t.Fatalf("parts = %#v", parts)
	}
	if gotBody["model"] != "gpt-5.5" || gotBody["stream"] != true {
		t.Fatalf("body = %#v", gotBody)
	}
}

func TestResponsesStreamsFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"read_file"}}`+"\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, `data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"path\":\"x\"}"}`+"\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client := provider.NewResponses(srv.URL+"/v1", "sk", srv.Client())
	var calls []provider.ToolCall
	err := client.StreamChat(context.Background(), "gpt-5.5", []runtime.Message{
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
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "read_file" {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0].Arguments != `{"path":"x"}` {
		t.Fatalf("args = %q", calls[0].Arguments)
	}
}
