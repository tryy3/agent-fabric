package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestOpenAIStreamsDeltas(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("accept = %q", r.Header.Get("Accept"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk-test", srv.Client())
	var parts []string
	err := client.StreamChat(context.Background(), "m", []runtime.Message{
		{Role: "user", Content: "hi"},
	}, func(ev provider.StreamEvent) error {
		if ev.Content != "" {
			parts = append(parts, ev.Content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if strings.Join(parts, "") != "Hello" {
		t.Fatalf("parts = %#v", parts)
	}
	if gotBody["model"] != "m" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	if gotBody["stream"] != true {
		t.Fatalf("stream = %v", gotBody["stream"])
	}
	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", gotBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok || message["role"] != "user" || message["content"] != "hi" {
		t.Fatalf("message = %#v", messages[0])
	}
}

func TestOpenAIHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk", srv.Client())
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "x"}}, func(provider.StreamEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v, want status and response body", err)
	}
}

func TestOpenAIEmptyAssistant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk", srv.Client())
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "x"}}, func(provider.StreamEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want empty", err)
	}
}

func TestOpenAICancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := provider.NewOpenAI(srv.URL+"/v1", "sk", srv.Client())
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.StreamChat(ctx, "m", []runtime.Message{{Role: "user", Content: "x"}}, func(provider.StreamEvent) error { return nil })
	}()
	<-started
	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context canceled", err)
	}
}

func TestOpenAIIncludeUsageAndReasoning(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hmm\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1,\"total_tokens\":4},\"timings\":{\"prompt_ms\":10,\"predicted_ms\":20,\"prompt_per_second\":100.5,\"predicted_per_second\":40.25}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk-test", srv.Client())
	var thoughts, contents []string
	var usage *provider.Usage
	var finish string
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "q"}}, func(ev provider.StreamEvent) error {
		if ev.Thought != "" {
			thoughts = append(thoughts, ev.Thought)
		}
		if ev.Content != "" {
			contents = append(contents, ev.Content)
		}
		if ev.Finish != "" {
			finish = ev.Finish
		}
		if ev.Usage != nil {
			usage = ev.Usage
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	opts, _ := gotBody["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Fatalf("stream_options = %#v", gotBody["stream_options"])
	}
	if strings.Join(thoughts, "") != "hmm" || strings.Join(contents, "") != "hi" {
		t.Fatalf("thoughts=%v contents=%v", thoughts, contents)
	}
	if finish != "stop" {
		t.Fatalf("finish = %q", finish)
	}
	if usage == nil || usage.PromptTokens == nil || *usage.PromptTokens != 3 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.PredictedPerSecond == nil || *usage.PredictedPerSecond != 40.25 {
		t.Fatalf("tok/s = %+v", usage.PredictedPerSecond)
	}
	if usage.Deltas != 1 {
		t.Fatalf("deltas = %d", usage.Deltas)
	}
	if usage.TTFTMs == nil || usage.ElapsedMs == nil {
		t.Fatal("expected plane TTFT and elapsed")
	}
}

func TestOpenAIOmitsMissingUsageFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := provider.NewOpenAI(srv.URL+"/v1", "sk-test", srv.Client())
	var usage *provider.Usage
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "q"}}, func(ev provider.StreamEvent) error {
		if ev.Usage != nil {
			usage = ev.Usage
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if usage == nil || usage.PromptTokens == nil || *usage.PromptTokens != 3 {
		t.Fatalf("prompt tokens = %+v", usage)
	}
	if usage.CompletionTokens != nil || usage.TotalTokens != nil {
		t.Fatalf("token fields should be omitted: completion=%v total=%v", usage.CompletionTokens, usage.TotalTokens)
	}
	if usage.PromptMs != nil || usage.PredictedMs != nil || usage.PromptPerSecond != nil || usage.PredictedPerSecond != nil {
		t.Fatalf("timing fields should be omitted: %+v", usage)
	}
	if usage.Deltas != 1 {
		t.Fatalf("deltas = %d", usage.Deltas)
	}
	if usage.TTFTMs == nil || usage.ElapsedMs == nil {
		t.Fatal("expected plane TTFT and elapsed")
	}
}

func TestOpenAIThinkingWithoutContentIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"only\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	client := provider.NewOpenAI(srv.URL+"/v1", "sk", srv.Client())
	err := client.StreamChat(context.Background(), "m", []runtime.Message{{Role: "user", Content: "x"}}, func(provider.StreamEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want empty", err)
	}
}
