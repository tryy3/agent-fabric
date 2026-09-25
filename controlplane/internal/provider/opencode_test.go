package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestOpenCodeRoutesByModelAndSetsHeaders(t *testing.T) {
	var paths []string
	var sessions []string
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		sessions = append(sessions, r.Header.Get("x-opencode-session"))
		agents = append(agents, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		switch {
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"cc\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"am\"}}\n\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
		case strings.HasSuffix(r.URL.Path, "/responses"):
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"rs\"}\n\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		flusher.Flush()
	}))
	defer srv.Close()

	oc, err := provider.NewOpenCode(catalog.TypeOpenCodeZen, srv.URL+"/v1", "sk", "sess_42", srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		model string
		want  string
		path  string
	}{
		{"deepseek-v4-flash", "cc", "/v1/chat/completions"},
		{"claude-opus-4-6", "am", "/v1/messages"},
		{"gpt-5.5", "rs", "/v1/responses"},
	}
	for _, tc := range cases {
		paths = nil
		sessions = nil
		agents = nil
		var got string
		err := oc.StreamChat(context.Background(), tc.model, []runtime.Message{
			{Role: "user", Content: "x"},
		}, provider.StreamChatOptions{}, func(ev provider.StreamEvent) error {
			got += ev.Content
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.model, err)
		}
		if got != tc.want {
			t.Fatalf("%s content = %q, want %q", tc.model, got, tc.want)
		}
		if len(paths) != 1 || paths[0] != tc.path {
			t.Fatalf("%s paths = %#v, want %q", tc.model, paths, tc.path)
		}
		if sessions[0] != "sess_42" || agents[0] != "agent-fabric/1.0" {
			t.Fatalf("%s headers session=%q ua=%q", tc.model, sessions[0], agents[0])
		}
	}
}
