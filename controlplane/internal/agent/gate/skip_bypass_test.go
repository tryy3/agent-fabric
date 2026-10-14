package gate

import (
	"context"
	"encoding/json"
	"testing"
)

// The skip bypass (the web tier, an explicit consult override) only reaches
// scorers that ask gate questions about the tool. With score and bands scorers
// alone the cascade behaves as it did before the web tier existed.
func TestSkipBypassNeedsAQuestionScorer(t *testing.T) {
	on := true
	consultRead := map[string]TierOverride{TierFileRead: {Consult: &on}}
	read := Request{ToolName: "read_file", Args: json.RawMessage(`{"path":"customers.csv"}`), ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker"}
	const (
		fastScore = iota
		fastQuestions
		deepBands
		deepQuestions
	)
	for _, tc := range []struct {
		name      string
		scorer    int
		overrides map[string]TierOverride
		req       Request
		skip      Risk
		wantCalls int
	}{
		{"web + score fast stays settled", fastScore, nil, webSearch, DefaultSkipAtOrBelow, 0},
		{"web + score fast stays settled at skip 0", fastScore, nil, webSearch, 0, 0},
		{"web + bands deep stays settled", deepBands, nil, webSearch, DefaultSkipAtOrBelow, 0},
		{"web + bands deep stays settled at skip 0", deepBands, nil, webSearch, 0, 0},
		{"web + questions fast is asked", fastQuestions, nil, webSearch, DefaultSkipAtOrBelow, 1},
		{"web + questions deep is asked", deepQuestions, nil, webSearch, DefaultSkipAtOrBelow, 1},
		{"stored file.read consult + score fast keeps the skip", fastScore, consultRead, read, DefaultSkipAtOrBelow, 0},
		{"stored file.read consult + bands deep keeps the skip", deepBands, consultRead, read, DefaultSkipAtOrBelow, 0},
		{"stored file.read consult + questions fast is asked", fastQuestions, consultRead, read, DefaultSkipAtOrBelow, 1},
		{"stored file.read consult + questions deep is asked", deepQuestions, consultRead, read, DefaultSkipAtOrBelow, 1},
	} {
		srv, _, s1calls := serveSystemOne(t, `{"answers":{"risk":{"score":2,"confidence":0.9},
			"q_data_leak":{"noul":0.05},"q_suspicious_host":{"noul":0.05},"q_planted_request":{"noul":0.05},
			"q_bulk_gathering":{"noul":0.05},"q_personal_data":{"noul":0.05}}}`)
		st := &recordingStreamer{answer: `{"band":"low","requested":false,"rationale":"x"}`}
		c := Cascade{Rules: Rules{Builtins: tc.overrides}, SkipAtOrBelow: tc.skip}
		switch tc.scorer {
		case fastScore:
			c.Fast = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScore}
		case fastQuestions:
			c.Fast = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}
		case deepBands:
			c.Deep = LLMScorer{Streamer: st, Model: "m", Style: StyleBands}
		case deepQuestions:
			st.answer = `{"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":false},"planted_request":{"yes":false},
				"bulk_gathering":{"yes":false},"personal_data":{"yes":false}},"rationale":"x"}`
			c.Deep = LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions}
		}
		d, err := c.Evaluate(context.Background(), tc.req)
		if err != nil {
			t.Fatal(err)
		}
		if calls := *s1calls + st.calls; calls != tc.wantCalls {
			t.Errorf("%s: scorer calls %d, want %d (%+v)", tc.name, calls, tc.wantCalls, d)
		}
		if tc.wantCalls == 0 && d.Risk != ruleRiskOf(t, tc.overrides, tc.req) {
			t.Errorf("%s: risk %d, want the rules' score", tc.name, d.Risk)
		}
	}
}

func ruleRiskOf(t *testing.T, overrides map[string]TierOverride, req Request) Risk {
	t.Helper()
	d, err := Rules{Builtins: overrides}.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return d.Risk
}
