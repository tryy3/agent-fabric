package gate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
)

func TestSystemOneScorerRequestAndMapping(t *testing.T) {
	var gotAuth, gotPath string
	var gotReq systemOneRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotReq)
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"risk":{"type":"score","score":6.4,"confidence":0.5}},"usage":{"input_tokens":120,"output_tokens":3}}`))
	}))
	defer srv.Close()

	var captured provider.HopCapture
	stats := &ScorerStats{}
	d, err := SystemOneScorer{
		BaseURL: srv.URL + "/", APIKey: "sk-test-key-123", Model: "jev-latest", Stats: stats,
		OnCapture: func(c provider.HopCapture) { captured = c },
	}.Evaluate(context.Background(), Request{
		ToolName: "run_command", Args: []byte(`{"command":["rm","x"]}`), UserIntent: "tidy up",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Risk != 7 || d.Kind != Allow || d.Source != "systemone:jev-latest" || !strings.Contains(d.Rationale, "6.40") {
		t.Fatalf("%+v", d)
	}
	if gotPath != "/v1/systemone" || gotAuth != "Bearer sk-test-key-123" || gotReq.Model != "jev-latest" {
		t.Fatalf("path=%q auth=%q req=%+v", gotPath, gotAuth, gotReq)
	}
	q := gotReq.Questions["risk"]
	if q.Type != "score" || len(q.Criteria) != 10 || !strings.Contains(gotReq.State["body"], "tidy up") {
		t.Fatalf("question = %+v state = %v", q, gotReq.State)
	}
	if stats.PromptTokens.Load() != 120 || stats.CompletionTokens.Load() != 3 || stats.Calls.Load() != 1 {
		t.Fatalf("stats %d %d", stats.PromptTokens.Load(), stats.CompletionTokens.Load())
	}
	if captured.URL == "" || captured.ReqHeaders.Get("Authorization") != "" || strings.Contains(string(captured.ReqBody), "sk-test-key-123") {
		t.Fatalf("capture leaks or is empty (url=%q auth=%q)", captured.URL, captured.ReqHeaders.Get("Authorization"))
	}
}

func TestSystemOneScoreBounds(t *testing.T) {
	for score, want := range map[string]Risk{"0": 1, "0.4": 1, "9": 10, "9.9": 10, "4.5": 6} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"risk":{"score":` + score + `}}}`))
		}))
		d, _ := SystemOneScorer{BaseURL: srv.URL}.Evaluate(context.Background(), Request{ToolName: "x", Args: []byte(`{}`)})
		srv.Close()
		if d.Risk != want {
			t.Errorf("score %s -> risk %d, want %d", score, d.Risk, want)
		}
	}
}

func TestSystemOneScorerFailsClosed(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"401":            func(w http.ResponseWriter, _ *http.Request) { http.Error(w, `{"error":"bad key"}`, 401) },
		"422":            func(w http.ResponseWriter, _ *http.Request) { http.Error(w, `{"detail":"invalid criteria"}`, 422) },
		"bad json":       func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`nope`)) },
		"missing answer": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"answers":{}}`)) },
	}
	for name, h := range cases {
		srv := httptest.NewServer(h)
		stats := &ScorerStats{}
		d, err := SystemOneScorer{BaseURL: srv.URL, Stats: stats}.Evaluate(context.Background(), Request{ToolName: "x", Args: []byte(`{}`)})
		srv.Close()
		if err != nil || d.Risk != DefaultFailRisk || d.RuleID != "systemone.fail_closed" || stats.Failures.Load() != 1 {
			t.Errorf("%s: %+v, %v", name, d, err)
		}
		if name == "401" && !strings.Contains(d.Rationale, "HTTP 401") {
			t.Errorf("rationale lacks status: %s", d.Rationale)
		}
	}
}
