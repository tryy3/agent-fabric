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

func TestResponsesSetsTopLevelInstructions(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	err := client.StreamChat(context.Background(), "gpt-5.5", []runtime.Message{
		{Role: "user", Content: "hi"},
	}, provider.StreamChatOptions{Instructions: "Be careful."}, func(provider.StreamEvent) error {
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if gotBody["instructions"] != "Be careful." {
		t.Fatalf("instructions = %#v", gotBody["instructions"])
	}
	input, ok := gotBody["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v", gotBody["input"])
	}
	first, ok := input[0].(map[string]any)
	if !ok || first["role"] == "developer" || first["role"] == "system" {
		t.Fatalf("expected user input without duplicated instructions, got %#v", input[0])
	}
}

func TestResponsesOmitsEmptyInstructions(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	err := client.StreamChat(context.Background(), "gpt-5.5", []runtime.Message{
		{Role: "user", Content: "hi"},
	}, provider.StreamChatOptions{}, func(provider.StreamEvent) error {
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if _, ok := gotBody["instructions"]; ok {
		t.Fatalf("expected instructions omitted, got %#v", gotBody["instructions"])
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

func TestResponsesRequestIncludesInference(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer srv.Close()

	temp := 0.5
	maxTok := 1024
	effort := "medium"
	client := provider.NewResponses(srv.URL+"/v1", "sk", srv.Client())
	err := client.StreamChat(context.Background(), "gpt-5.5", []runtime.Message{
		{Role: "user", Content: "hi"},
	}, provider.StreamChatOptions{
		Temperature:     &temp,
		MaxTokens:       &maxTok,
		ReasoningEffort: &effort,
	}, func(provider.StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if gotBody["temperature"] != 0.5 {
		t.Fatalf("temperature = %#v", gotBody["temperature"])
	}
	if gotBody["max_output_tokens"] != float64(1024) {
		t.Fatalf("max_output_tokens = %#v", gotBody["max_output_tokens"])
	}
	reasoning, ok := gotBody["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "medium" {
		t.Fatalf("reasoning = %#v", gotBody["reasoning"])
	}
}

func TestResponsesCaptureContainsInstructions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Yo\"}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	var captured map[string]any
	client := provider.NewResponses(srv.URL+"/v1", "sk", srv.Client())
	err := client.StreamChat(context.Background(), "gpt-5.5", []runtime.Message{
		{Role: "user", Content: "hi"},
	}, provider.StreamChatOptions{
		Instructions: "Be careful.",
		OnCapture: func(hop provider.HopCapture) {
			_ = json.Unmarshal(hop.ReqBody, &captured)
		},
	}, func(provider.StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if captured["instructions"] != "Be careful." {
		t.Fatalf("captured instructions = %#v", captured["instructions"])
	}
}
