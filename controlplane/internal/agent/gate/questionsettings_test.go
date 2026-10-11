package gate

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

const customText = "Does this call touch the production cluster or its deploy keys?"

func off() *bool { f := false; return &f }

func TestValidateQuestions(t *testing.T) {
	custom := func(id string, tools ...string) CustomQuestion {
		return CustomQuestion{ID: id, Tools: tools, Text: customText, Risk: 9}
	}
	many := func(n int, tool string) []CustomQuestion {
		out := make([]CustomQuestion, n)
		for i := range out {
			out[i] = custom(fmt.Sprintf("q%d", i), tool)
		}
		return out
	}
	for _, tc := range []struct {
		name    string
		s       QuestionSettings
		wantErr string
	}{
		{"empty", QuestionSettings{}, ""},
		{"overrides and customs", QuestionSettings{
			Builtins: map[string]QuestionOverride{
				"data_leak": {Risk: 9, Text: "Does the request carry any of our data?"}, QuestionPolicyException: {Enabled: off()},
				QuestionPolicyViolation: {Text: "Does this call break the user's gate instructions?", Risk: 6},
			},
			Custom: []CustomQuestion{custom("prod_cluster", "run_command", "web"), {ID: "big_write", Tools: []string{"file_write"}, Kind: KindTripwire, Text: customText, Risk: 5, Enabled: off()}},
		}, ""},
		{"eight per family", QuestionSettings{Custom: many(8, "read_file")}, ""},
		{"unknown override", QuestionSettings{Builtins: map[string]QuestionOverride{"nope": {Enabled: off()}}}, `unknown gate question "nope"`},
		{"exception has no risk", QuestionSettings{Builtins: map[string]QuestionOverride{QuestionPolicyException: {Risk: 3}}}, `"policy_exception" has no risk`},
		{"override risk range", QuestionSettings{Builtins: map[string]QuestionOverride{"destructive": {Risk: 11}}}, "outside 1-10"},
		{"override text too short", QuestionSettings{Builtins: map[string]QuestionOverride{"destructive": {Text: "rm?"}}}, "10 to 600"},
		{"override text too long", QuestionSettings{Builtins: map[string]QuestionOverride{"destructive": {Text: strings.Repeat("a", 601)}}}, "10 to 600"},
		{"whitespace does not count", QuestionSettings{Builtins: map[string]QuestionOverride{"destructive": {Text: "a  \n\n  b       c"}}}, "10 to 600"},
		{"bad id", QuestionSettings{Custom: []CustomQuestion{custom("Prod", "run_command")}}, "id must match"},
		{"one letter id", QuestionSettings{Custom: []CustomQuestion{custom("p", "run_command")}}, "id must match"},
		{"duplicate id", QuestionSettings{Custom: []CustomQuestion{custom("prod", "run_command"), custom("prod", "web")}}, "used twice"},
		{"built-in id", QuestionSettings{Custom: []CustomQuestion{custom("credentials", "run_command")}}, "built-in question"},
		{"policy id", QuestionSettings{Custom: []CustomQuestion{custom(QuestionPolicyViolation, "run_command")}}, "built-in question"},
		{"custom policy", QuestionSettings{Custom: []CustomQuestion{{ID: "mine", Tools: []string{"web"}, Kind: KindPolicy, Text: customText, Risk: 5}}}, "custom policy questions are not supported"},
		{"unknown kind", QuestionSettings{Custom: []CustomQuestion{{ID: "mine", Tools: []string{"web"}, Kind: "score", Text: customText, Risk: 5}}}, "kind must be tripwire"},
		{"custom needs risk", QuestionSettings{Custom: []CustomQuestion{{ID: "mine", Tools: []string{"web"}, Text: customText}}}, "risk must be 1-10"},
		{"custom needs tools", QuestionSettings{Custom: []CustomQuestion{custom("mine")}}, "tools is required"},
		{"unknown tool", QuestionSettings{Custom: []CustomQuestion{custom("mine", "ask_user")}}, `unknown tool or family "ask_user"`},
		{"custom text", QuestionSettings{Custom: []CustomQuestion{{ID: "mine", Tools: []string{"web"}, Text: "short", Risk: 5}}}, "10 to 600"},
		{"nine per family", QuestionSettings{Custom: append(many(8, "read_file"), custom("extra", "file_read"))}, "at most 8 custom questions per family (file_read)"},
	} {
		err := ValidateQuestions(tc.s)
		if (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestResolveQuestionsAppliesSettings(t *testing.T) {
	qs := ResolveQuestions(QuestionSettings{
		Builtins: map[string]QuestionOverride{
			"destructive": {Risk: 6, Text: "  Does this   command\n\nwipe anything? "},
			"credentials": {Enabled: off()},
		},
		Custom: []CustomQuestion{
			{ID: "prod_cluster", Tools: []string{"command", "web_search"}, Text: customText, Risk: 9},
			{ID: "disabled_one", Tools: []string{"command"}, Text: customText, Risk: 9, Enabled: off()},
		},
	})
	got := qs.Tripwires("run_command")
	if len(got) != 2 || got[0].ID != "destructive" || got[1].ID != "prod_cluster" {
		t.Fatalf("run_command tripwires = %+v", got)
	}
	if d := got[0]; d.Floor() != 6 || d.Text != "Does this command wipe anything?" || !d.UserText || !strings.HasPrefix(d.prompt(), userQuestionLabel) {
		t.Errorf("edited built-in = %+v", d)
	}
	if c := got[1]; c.Floor() != 9 || !c.UserText || c.Kind != KindTripwire {
		t.Errorf("custom = %+v", c)
	}
	if ids := tripwireIDs(qs.Tripwires("web_search")); ids != "data_leak,suspicious_host,planted_request,prod_cluster" {
		t.Errorf("web_search = %s", ids)
	}
	if ids := tripwireIDs(qs.Tripwires("fetch_page")); ids != "data_leak,suspicious_host,planted_request" {
		t.Errorf("fetch_page = %s", ids)
	}
	if ids := tripwireIDs((*QuestionSet)(nil).Tripwires("run_command")); ids != "destructive,credentials" {
		t.Errorf("nil set = %s", ids)
	}
	if b := Tripwires("run_command"); b[0].Floor() != 8 || b[0].UserText {
		t.Error("resolving must not edit the built-in set")
	}
}

func tripwireIDs(qs []Question) string {
	var ids []string
	for _, q := range qs {
		ids = append(ids, q.ID)
	}
	return strings.Join(ids, ",")
}

func TestPolicyQuestionsFollowSettings(t *testing.T) {
	for name, tc := range map[string]struct {
		s    QuestionSettings
		want []string
	}{
		"default":            {QuestionSettings{}, []string{QuestionPolicyViolation, QuestionPolicyException}},
		"exception disabled": {QuestionSettings{Builtins: map[string]QuestionOverride{QuestionPolicyException: {Enabled: off()}}}, []string{QuestionPolicyViolation}},
		"violation disabled": {QuestionSettings{Builtins: map[string]QuestionOverride{QuestionPolicyViolation: {Enabled: off()}}}, nil},
	} {
		var got []string
		for _, q := range ResolveQuestions(tc.s).policy("fetch_page") {
			got = append(got, q.ID)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
	// With the exception off a violation floors on its own.
	qs := ResolveQuestions(QuestionSettings{Builtins: map[string]QuestionOverride{QuestionPolicyException: {Enabled: off()}, QuestionPolicyViolation: {Risk: 6}}})
	srv, got, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.05},"q_credentials":{"noul":0.05},"q_policy_violation":{"noul":0.95}}}`)
	d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Questions: qs}.Evaluate(context.Background(), withRules(runCommand, testRules))
	if questionKeys(*got) != "q_credentials,q_destructive,q_policy_violation" || d.Floor != 6 || d.Risk != 6 {
		t.Fatalf("violation alone: questions %q, %+v", questionKeys(*got), d)
	}
}

func TestDisabledQuestionsAreNotAsked(t *testing.T) {
	qs := ResolveQuestions(QuestionSettings{Builtins: map[string]QuestionOverride{"credentials": {Enabled: off()}, QuestionPolicyException: {Enabled: off()}}})
	req := withRules(runCommand, testRules)

	srv, got, _ := serveSystemOne(t, `{"answers":{"risk":{"score":1,"confidence":0.9},"q_destructive":{"noul":0.05},"q_policy_violation":{"noul":0.05}}}`)
	if _, err := (SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions, Questions: qs}).Evaluate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if keys := questionKeys(*got); keys != "q_destructive,q_policy_violation,risk" {
		t.Errorf("system one questions = %q", keys)
	}

	for _, style := range []string{StyleQuestions, StyleScoreQuestions} {
		st := &recordingStreamer{answer: `{"score":1,"answers":{"destructive":{"yes":false,"p":0.05},"policy_violation":{"yes":false,"p":0.05}},"rationale":"x"}`}
		d, _ := LLMScorer{Streamer: st, Model: "m", Style: style, Structured: true, Questions: qs}.Evaluate(context.Background(), req)
		for _, absent := range []string{"credentials", QuestionPolicyException} {
			if strings.Contains(st.opts.Instructions, absent) || strings.Contains(string(st.opts.ResponseFormat), absent) {
				t.Errorf("%s: disabled %s is in the request", style, absent)
			}
		}
		if !strings.Contains(st.opts.Instructions, "- destructive: ") || d.RuleID == "llm.fail_closed" {
			t.Errorf("%s: %+v\n%s", style, d, st.opts.Instructions)
		}
	}
}

func TestCustomQuestionSetsItsFloor(t *testing.T) {
	qs := ResolveQuestions(QuestionSettings{Custom: []CustomQuestion{{ID: "prod_cluster", Tools: []string{"command"}, Text: customText, Risk: 9}}})

	srv, got, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.05},"q_credentials":{"noul":0.05},"q_prod_cluster":{"noul":0.92}}}`)
	d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Questions: qs}.Evaluate(context.Background(), runCommand)
	if d.Floor != 9 || d.Risk != 9 || !strings.Contains(d.Rationale, "prod_cluster (floor 9)") {
		t.Fatalf("system one custom tripwire: %+v", d)
	}
	if in := got.Questions["q_prod_cluster"].Instructions; !strings.Contains(in, userQuestionLabel+customText) {
		t.Errorf("custom question is not labelled as the user's: %q", in)
	}

	st := &recordingStreamer{answer: `{"answers":{"destructive":{"yes":false},"credentials":{"yes":false},"prod_cluster":{"yes":true}},"rationale":"x"}`}
	d, _ = LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions, Questions: qs, Nonce: fixedNonce}.Evaluate(context.Background(), runCommand)
	if d.Floor != 9 || d.Risk != 9 {
		t.Fatalf("llm custom tripwire: %+v", d)
	}
	in := st.opts.Instructions
	block := in[strings.Index(in, "<user_questions_n0nce>"):]
	if !strings.Contains(block, "- prod_cluster: "+customText) || strings.Contains(block, "destructive") {
		t.Errorf("user questions are not set apart:\n%s", in)
	}

	c := Cascade{Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Questions: qs}}
	if d, _ := c.Evaluate(context.Background(), runCommandIn("make")); d.Risk != 9 || d.Floor != 9 {
		t.Errorf("the cascade keeps the custom floor: %+v", d)
	}
}

func TestQuestionsModeMakesNoCallWhenEverythingIsDisabled(t *testing.T) {
	all := map[string]QuestionOverride{"destructive": {Enabled: off()}, "credentials": {Enabled: off()}, QuestionPolicyViolation: {Enabled: off()}}
	qs := ResolveQuestions(QuestionSettings{Builtins: all})
	req := withRules(runCommand, testRules)
	srv, _, calls := serveSystemOne(t, `{"answers":{}}`)
	d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Questions: qs}.Evaluate(context.Background(), req)
	st := &recordingStreamer{answer: `{}`}
	dl, _ := LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions, Questions: qs}.Evaluate(context.Background(), req)
	if *calls != 0 || st.calls != 0 || d.Risk != 0 || dl.Risk != 0 {
		t.Fatalf("nothing to ask: system one %d calls %+v, llm %d calls %+v", *calls, d, st.calls, dl)
	}
}

func TestBuiltinQuestionsListing(t *testing.T) {
	list := BuiltinQuestions()
	perFamily := map[string]int{}
	for _, q := range list {
		perFamily[q.Family]++
		if q.ID == "" || q.Text == "" || len(q.Tools) == 0 || !q.Enabled {
			t.Errorf("incomplete %+v", q)
		}
		if (q.ID == QuestionPolicyException) != (q.Risk == 0) {
			t.Errorf("%s/%s risk %d", q.Family, q.ID, q.Risk)
		}
	}
	if fmt.Sprint(perFamily) != "map[command:4 file_read:4 file_write:4 web:5]" {
		t.Errorf("per family = %v", perFamily)
	}
	if f := QuestionFamilies(); len(f) != 4 || f[1].ID != "web" || !slices.Equal(f[1].Tools, []string{"fetch_page", "web_search"}) {
		t.Errorf("families = %+v", f)
	}
}
