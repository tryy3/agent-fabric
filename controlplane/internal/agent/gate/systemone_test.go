package gate

import (
	"context"
	"encoding/json"
	"fmt"
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
		BaseURL: srv.URL + "/", APIKey: "sk-test-key-123", Model: "jev-latest", Stats: stats, Strategy: StrategyScore,
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
	if q.Type != "score" || len(q.Criteria) != 10 || !strings.Contains(fmt.Sprint(gotReq.State), "tidy up") {
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
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScore}.Evaluate(context.Background(), Request{ToolName: "x", Args: []byte(`{}`)})
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

func TestSystemOneBaseURLTolerance(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		http.Error(w, "Method Not Allowed", 405)
	}))
	defer srv.Close()
	var rationale string
	for _, base := range []string{srv.URL, srv.URL + "/v1", srv.URL + "/v1/"} {
		d, _ := SystemOneScorer{BaseURL: base}.Evaluate(context.Background(), Request{ToolName: "x", Args: []byte(`{}`)})
		rationale = d.Rationale
	}
	for _, p := range paths {
		if p != "/v1/systemone" {
			t.Errorf("path = %q", p)
		}
	}
	if !strings.Contains(rationale, "POST "+srv.URL+"/v1/systemone: HTTP 405") {
		t.Errorf("error should name the URL: %s", rationale)
	}
}

func TestSystemOneRationaleShowsDistribution(t *testing.T) {
	for name, probs := range map[string]string{
		"jev map":    `{"0":0.5,"3":0.25,"9":0.25}`,
		"laya array": `[0.5,0,0,0.25,0,0,0,0,0,0.25]`,
	} {
		got := systemOneRationale(3.2, 0.4, json.RawMessage(probs))
		for _, want := range []string{"expected level 3.20", "confidence 0.40", "0:0.50", "3:0.25", "9:0.25", "1:0.00"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q lacks %q", name, got, want)
			}
		}
	}
	if got := systemOneRationale(3.2, 0.4, nil); strings.Contains(got, "p(level)") {
		t.Errorf("no probabilities, no distribution: %s", got)
	}
}

// questionServer answers every yes/no question with the probability in p
// (default 0) and records the request.
func questionServer(t *testing.T, p map[string]float64, got *systemOneRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req systemOneRequest
		_ = json.Unmarshal(raw, &req)
		if got != nil {
			*got = req
		}
		answers := map[string]any{}
		for k := range req.Questions {
			answers[k] = map[string]any{"type": "noul", "noul": p[k]}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers, "usage": map[string]int{"input_tokens": 10}})
	}))
}

func TestSystemOneQuestionsStrategy(t *testing.T) {
	cases := []struct {
		name   string
		p      map[string]float64
		intent string
		want   Risk
	}{
		{"routine read", map[string]float64{"routine": 1}, "", 1},
		{"unrecognised call", map[string]float64{"routine": 0}, "", 4},
		{"destructive", map[string]float64{"routine": 0, "destroy": 1}, "", 8},
		{"half sure of exfiltration", map[string]float64{"routine": 0, "send": 0.5}, "", 5},
		{"probable exfiltration is cancelled", map[string]float64{"routine": 0, "send": 0.8}, "", 9},
		{"wiping the system", map[string]float64{"routine": 0, "system": 0.97, "destroy": 0.97}, "", 10},
		{"unlikely hazards are ignored", map[string]float64{"routine": 1, "send": 0.2, "escape": 0.1}, "", 1},
		{"reverse shell", map[string]float64{"routine": 0, "escape": 0.95, "send": 0.3}, "", 10},
		{"opaque", map[string]float64{"routine": 0.1, "opaque": 1}, "", 6},
		{"unrequested bumps", map[string]float64{"routine": 0, "requested": 0.1}, "tidy up", 5},
		{"requested does not bump", map[string]float64{"routine": 0, "requested": 0.9}, "tidy up", 4},
		{"no intent, no bump", map[string]float64{"routine": 0}, "", 4},
		{"routine unrequested stays safe", map[string]float64{"routine": 1, "requested": 0}, "tidy up", 1},
	}
	for _, tc := range cases {
		var req systemOneRequest
		srv := questionServer(t, tc.p, &req)
		d, err := SystemOneScorer{BaseURL: srv.URL}.Evaluate(context.Background(),
			Request{ToolName: "run_command", Args: []byte(`{}`), UserIntent: tc.intent})
		srv.Close()
		if err != nil || d.Risk != tc.want || d.RuleID != "systemone.score" {
			t.Errorf("%s: risk=%d rule=%s (%q), want %d", tc.name, d.Risk, d.RuleID, d.Rationale, tc.want)
		}
		wantQuestions := 8
		if tc.intent != "" {
			wantQuestions = 9
		}
		if len(req.Questions) != wantQuestions || req.Questions["escape"].Type != "noul" {
			t.Errorf("%s: questions = %v", tc.name, req.Questions)
		}
	}
}

func TestSystemOneQuestionsMissingAnswerFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"routine":{"noul":0.5}}}`))
	}))
	defer srv.Close()
	d, _ := SystemOneScorer{BaseURL: srv.URL}.Evaluate(context.Background(), Request{ToolName: "x", Args: []byte(`{}`)})
	if d.RuleID != "systemone.fail_closed" || !strings.Contains(d.Rationale, "no yes/no answer") {
		t.Fatalf("%+v", d)
	}
}

func TestSystemOneStateFormat(t *testing.T) {
	for format, check := range map[string]func(any) bool{
		"": func(s any) bool {
			m, ok := s.(map[string]any)
			return ok && strings.Contains(m["body"].(string), "run_command")
		},
		StateObject: func(s any) bool {
			m, ok := s.(map[string]any)
			return ok && strings.Contains(m["body"].(string), "run_command")
		},
		StateText: func(s any) bool { str, ok := s.(string); return ok && strings.Contains(str, "run_command") },
	} {
		var raw map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &raw)
			http.Error(w, "stop", 400)
		}))
		_, _ = SystemOneScorer{BaseURL: srv.URL, StateFormat: format}.Evaluate(context.Background(), Request{ToolName: "run_command", Args: []byte(`{}`)})
		srv.Close()
		if !check(raw["state"]) {
			t.Errorf("format %q: state = %#v", format, raw["state"])
		}
	}
}

func TestSystemOneScoreUsesMajorityLevelAndReportsConfidence(t *testing.T) {
	for name, tc := range map[string]struct {
		answer string
		want   Risk
	}{
		"majority level beats the mean": {`{"score":1.64,"confidence":0.74,"probabilities":{"1":0.69,"3":0.29,"4":0.02}}`, 2},
		"no majority uses the mean":     {`{"score":8.06,"confidence":0.62,"probabilities":{"7":0.36,"8":0.2,"9":0.44}}`, 9},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"risk":` + tc.answer + `}}`))
		}))
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScore}.Evaluate(context.Background(), Request{ToolName: "x", Args: []byte(`{}`)})
		srv.Close()
		if d.Risk != tc.want || d.Confidence == 0 {
			t.Errorf("%s: risk %d confidence %v, want risk %d", name, d.Risk, d.Confidence, tc.want)
		}
	}
}

func TestSystemOneQuestionsConfidence(t *testing.T) {
	srv := questionServer(t, map[string]float64{"routine": 0.95, "send": 0.46}, nil)
	defer srv.Close()
	d, _ := SystemOneScorer{BaseURL: srv.URL}.Evaluate(context.Background(), Request{ToolName: "run_command", Args: []byte(`{}`)})
	if d.Confidence > 0.1 {
		t.Fatalf("an undecided answer must lower confidence: %v", d.Confidence)
	}
}

func TestSystemOneBandsStrategy(t *testing.T) {
	cases := []struct {
		name      string
		level     string
		requested float64
		intent    string
		want      Risk
	}{
		{"safe", "0.1", 0.5, "", 1},
		{"high, no request known", "3", 0.5, "", 7},
		{"high, requested", "3", 0.9, "clean up", 5},
		{"high, undecided request", "3", 0.5, "clean up", 7},
		{"low, not requested", "1", 0.1, "add a README", 4},
		{"cancel", "4", 0.9, "do it", 9},
	}
	for _, tc := range cases {
		var got systemOneRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &got)
			_, _ = fmt.Fprintf(w, `{"answers":{"risk":{"score":%s,"confidence":0.8},"requested":{"noul":%v}}}`, tc.level, tc.requested)
		}))
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyBands}.Evaluate(context.Background(),
			Request{ToolName: "run_command", Args: []byte(`{}`), UserIntent: tc.intent})
		srv.Close()
		if d.Risk != tc.want || d.Confidence != 0.8 || d.RuleID != "systemone.score" {
			t.Errorf("%s: risk %d conf %v (%s), want %d", tc.name, d.Risk, d.Confidence, d.Rationale, tc.want)
		}
		if q := got.Questions["risk"]; q.Type != "score" || len(q.Criteria) != 5 {
			t.Errorf("%s: band question = %+v", tc.name, q)
		}
		if _, asked := got.Questions["requested"]; asked != (tc.intent != "") {
			t.Errorf("%s: requested question asked = %v", tc.name, asked)
		}
	}
}
