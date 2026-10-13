package gate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// serveSystemOne answers every request with body and records the last one.
func serveSystemOne(t *testing.T, body string) (*httptest.Server, *systemOneRequest, *int) {
	t.Helper()
	var got systemOneRequest
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &calls
}

func questionKeys(r systemOneRequest) string {
	keys := make([]string, 0, len(r.Questions))
	for k := range r.Questions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

var runCommand = Request{ToolName: "run_command", Args: []byte(`{"command":["rm","x"]}`)}

func TestSystemOneStrategiesBuildTheRightRequest(t *testing.T) {
	const answers = `{"answers":{"risk":{"score":2,"confidence":0.9},"requested":{"noul":0.5},"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`
	for strategy, want := range map[string]string{
		StrategyScore:          "risk",
		StrategyBands:          "requested,risk",
		StrategyQuestions:      "q_credentials,q_destructive,q_remote_code_or_exfil",
		StrategyScoreQuestions: "q_credentials,q_destructive,q_remote_code_or_exfil,risk",
	} {
		srv, got, calls := serveSystemOne(t, answers)
		req := runCommand
		req.UserIntent = "tidy up"
		if _, err := (SystemOneScorer{BaseURL: srv.URL, Strategy: strategy}).Evaluate(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if *calls != 1 || questionKeys(*got) != want {
			t.Errorf("%s: %d calls, questions %q, want %q", strategy, *calls, questionKeys(*got), want)
		}
		for k, q := range got.Questions {
			if strings.HasPrefix(k, "q_") && (q.Type != "noul" || len(q.Criteria) != 0 || q.Instructions == "") {
				t.Errorf("%s: %s = %+v, want a noul question with instructions and no criteria", strategy, k, q)
			}
		}
	}
	srv, got, _ := serveSystemOne(t, answers)
	_, _ = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), runCommand)
	if q := got.Questions["q_destructive"]; !strings.Contains(q.Instructions, "irreversibly destroy") || !strings.Contains(q.Instructions, "GitOps") {
		t.Errorf("destructive question lacks its wording: %q", q.Instructions)
	}
}

func TestSystemOneQuestionsAbstainsWithoutTripwire(t *testing.T) {
	srv, _, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.05},"q_credentials":{"noul":0.5},"q_remote_code_or_exfil":{"noul":0.1}}}`)
	stats := &ScorerStats{}
	d, err := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Stats: stats, Name: "fast"}.Evaluate(context.Background(), runCommand)
	if err != nil {
		t.Fatal(err)
	}
	if d.Risk != 0 || d.Floor != 0 || d.Kind != Allow || d.RuleID != "systemone.score" {
		t.Fatalf("no tripwire fired, the scorer abstains: %+v", d)
	}
	for _, want := range []string{"destructive 0.05", "credentials 0.50"} {
		if !strings.Contains(d.Rationale, want) {
			t.Errorf("rationale %q lacks %q", d.Rationale, want)
		}
	}
	if strings.Contains(d.Rationale, "tripwire") || stats.Calls.Load() != 1 {
		t.Errorf("an undecided answer counts as nothing: %q", d.Rationale)
	}
	cd, _ := Cascade{Rules: scored("rules", 5, 0), Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Name: "fast"}}.Evaluate(context.Background(), runCommand)
	if cd.Risk != 5 || len(cd.Scores) != 2 || cd.Scores[1].Source != "fast" {
		t.Fatalf("the cascade keeps the rules' score and lists the scorer: %+v", cd)
	}
}

func TestSystemOneTripwireThresholds(t *testing.T) {
	for p, fires := range map[string]bool{"0.69": false, "0.7": false, "0.71": true, "0.5": false, "0.3": false, "0.29": false, "1": true} {
		srv, _, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":`+p+`},"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`)
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), runCommand)
		if fired := d.Floor == 8 && d.Risk == 8; fired != fires {
			t.Errorf("p=%s: risk %d floor %d, fires = %v", p, d.Risk, d.Floor, fires)
		}
	}
}

func TestSystemOneQuestionsFiredSetsFloorAndConfidence(t *testing.T) {
	for name, tc := range map[string]struct {
		answer string
		want   float64
	}{
		"reported confidence": {`{"noul":0.9,"confidence":0.55}`, 0.55},
		"derived from p":      {`{"noul":0.9}`, 0.8},
	} {
		srv, _, _ := serveSystemOne(t, `{"answers":{"q_destructive":`+tc.answer+`,"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`)
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), runCommand)
		if d.Risk != 8 || d.Floor != 8 || d.Confidence < tc.want-1e-9 || d.Confidence > tc.want+1e-9 {
			t.Errorf("%s: %+v", name, d)
		}
		if !strings.Contains(d.Rationale, "tripwire destructive (floor 8)") {
			t.Errorf("%s: rationale %q", name, d.Rationale)
		}
	}
}

func TestSystemOneScoreQuestionsRaisesToFloor(t *testing.T) {
	cases := map[string]struct {
		body             string
		wantRisk, wantFl Risk
	}{
		"tripwire raises a low score": {`{"answers":{"risk":{"score":2,"confidence":0.9},"q_destructive":{"noul":0.95},"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`, 8, 8},
		"high score stays":            {`{"answers":{"risk":{"score":7.6,"confidence":0.9},"q_destructive":{"noul":0.95},"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`, 9, 8},
		"no tripwire, score as is":    {`{"answers":{"risk":{"score":2,"confidence":0.9},"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.5},"q_remote_code_or_exfil":{"noul":0.1}}}`, 3, 0},
	}
	for name, tc := range cases {
		srv, _, _ := serveSystemOne(t, tc.body)
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions}.Evaluate(context.Background(), runCommand)
		if d.Risk != tc.wantRisk || d.Floor != tc.wantFl || d.Confidence != 0.9 {
			t.Errorf("%s: risk %d floor %d conf %v (%s)", name, d.Risk, d.Floor, d.Confidence, d.Rationale)
		}
		if !strings.Contains(d.Rationale, "expected level") || !strings.Contains(d.Rationale, "p(yes)") {
			t.Errorf("%s: rationale %q", name, d.Rationale)
		}
	}
}

func TestSystemOneQuestionsNeedsNoCallForToolsWithoutQuestions(t *testing.T) {
	srv, _, calls := serveSystemOne(t, `{"answers":{}}`)
	stats := &ScorerStats{}
	d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Stats: stats}.Evaluate(context.Background(), Request{ToolName: "ask_user", Args: []byte(`{}`)})
	if *calls != 0 || stats.Calls.Load() != 0 || d.Risk != 0 || d.Rationale != "" {
		t.Fatalf("calls=%d stats=%d decision=%+v", *calls, stats.Calls.Load(), d)
	}
	// score+questions on such a tool is the plain score question.
	srv, got, _ := serveSystemOne(t, `{"answers":{"risk":{"score":1}}}`)
	d, _ = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions}.Evaluate(context.Background(), Request{ToolName: "ask_user", Args: []byte(`{}`)})
	if questionKeys(*got) != "risk" || d.Risk != 2 || d.Floor != 0 {
		t.Fatalf("questions %q decision %+v", questionKeys(*got), d)
	}
}

func TestSystemOneQuestionsFailClosed(t *testing.T) {
	cases := map[string]struct {
		strategy string
		handler  http.HandlerFunc
	}{
		"questions 500": {StrategyQuestions, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", 500) }},
		"questions missing": {StrategyQuestions, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"q_destructive":{"noul":0.1}}}`))
		}},
		"questions bad json": {StrategyQuestions, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`nope`)) }},
		"combined no noul": {StrategyScoreQuestions, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"risk":{"score":1},"q_destructive":{},"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`))
		}},
		"combined no risk": {StrategyScoreQuestions, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_remote_code_or_exfil":{"noul":0.1}}}`))
		}},
		"combined 401": {StrategyScoreQuestions, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", 401) }},
	}
	for name, tc := range cases {
		srv := httptest.NewServer(tc.handler)
		stats := &ScorerStats{}
		d, err := SystemOneScorer{BaseURL: srv.URL, Strategy: tc.strategy, Stats: stats}.Evaluate(context.Background(), runCommand)
		srv.Close()
		if err != nil || d.Risk != DefaultFailRisk || d.RuleID != "systemone.fail_closed" || d.Floor != 0 || stats.Failures.Load() != 1 {
			t.Errorf("%s: %+v, %v", name, d, err)
		}
	}
}
