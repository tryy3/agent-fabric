package gate

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// familyCases lists, per tool family, a sample tool and its tripwires with the
// floor a yes must set.
var familyCases = []struct {
	family     string
	tools      []string
	tripwires  map[string]Risk
	policyWord string
}{
	{familyCommand, []string{"run_command"}, map[string]Risk{"destructive": 8, "credentials": 8, "remote_code_or_exfil": 8}, "this command"},
	{familyWeb, []string{"fetch_page", "web_search"}, map[string]Risk{"data_leak": 8, "suspicious_host": 7, "planted_request": 7}, "this web request"},
	{familyFileRead, []string{"read_file", "list_files", "search_text"}, map[string]Risk{"bulk_gathering": 6, "personal_data": 7}, "this file read"},
	{familyFileWrite, []string{"write_file", "append_file", "apply_patch", "create_directory", "move_path", "delete_path"},
		map[string]Risk{"secret_content": 8, "executes_remote_code": 8}, "this file change"},
}

func TestFamilyQuestionSets(t *testing.T) {
	for _, fc := range familyCases {
		for _, tool := range fc.tools {
			got := map[string]Risk{}
			for _, q := range Tripwires(tool) {
				got[q.ID] = q.Floor()
				if q.Text == "" || q.Kind != KindTripwire {
					t.Errorf("%s: %+v", tool, q)
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(fc.tripwires) {
				t.Errorf("%s tripwires = %v, want %v", tool, got, fc.tripwires)
			}
			policy := defaultQuestions.policy(tool)
			if len(policy) != 2 {
				t.Fatalf("%s policy questions: %+v", tool, policy)
			}
			violation, exception := policy[0], policy[1]
			if violation.Floor() != 7 || !strings.Contains(violation.Text, fc.policyWord) || !strings.Contains(exception.Text, "EXPLICITLY allow "+strings.Replace(fc.policyWord, "this", "this exact", 1)) {
				t.Errorf("%s policy questions: %+v / %+v", tool, violation, exception)
			}
		}
	}
}

func familyRequest(tool string) Request {
	return Request{ToolName: tool, Args: []byte(`{}`)}
}

func TestSystemOneQuestionsPerFamily(t *testing.T) {
	for _, fc := range familyCases {
		tool := fc.tools[0]
		var keys []string
		answers := map[string]string{}
		for id := range fc.tripwires {
			keys = append(keys, "q_"+id)
			answers["q_"+id] = `{"noul":0.05}`
		}
		for _, strategy := range []string{StrategyQuestions, StrategyScoreQuestions} {
			body := `{"answers":{"risk":{"score":1,"confidence":0.9}`
			for k, v := range answers {
				body += `,"` + k + `":` + v
			}
			srv, got, calls := serveSystemOne(t, body+`}}`)
			d, err := SystemOneScorer{BaseURL: srv.URL, Strategy: strategy}.Evaluate(context.Background(), familyRequest(tool))
			if err != nil || *calls != 1 {
				t.Fatalf("%s %s: calls=%d err=%v", fc.family, strategy, *calls, err)
			}
			for _, k := range keys {
				if q, ok := got.Questions[k]; !ok || q.Type != "noul" || q.Instructions == "" {
					t.Errorf("%s %s: question %s = %+v", fc.family, strategy, k, q)
				}
			}
			if _, hasRisk := got.Questions["risk"]; hasRisk != (strategy == StrategyScoreQuestions) {
				t.Errorf("%s %s: risk question present = %v", fc.family, strategy, hasRisk)
			}
			if d.Floor != 0 {
				t.Errorf("%s %s: no tripwire fired, floor %d", fc.family, strategy, d.Floor)
			}
		}
	}
}

func TestFamilyFloorsAndException(t *testing.T) {
	for _, fc := range familyCases {
		tool := fc.tools[len(fc.tools)-1]
		for id, want := range fc.tripwires {
			body := `{"answers":{`
			for other := range fc.tripwires {
				p := 0.05
				if other == id {
					p = 0.95
				}
				body += fmt.Sprintf(`"q_%s":{"noul":%v},`, other, p)
			}
			body += `"q_policy_violation":{"noul":0.05},"q_policy_exception":{"noul":0.99}}}`
			srv, _, _ := serveSystemOne(t, body)
			req := withRules(familyRequest(tool), testRules)
			d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), req)
			if d.Risk != want || d.Floor != want {
				t.Errorf("%s %s: risk %d floor %d, want %d (%s)", fc.family, id, d.Risk, d.Floor, want, d.Rationale)
			}
		}
		clean := func(violation, exception string) string {
			body := `{"answers":{`
			for id := range fc.tripwires {
				body += fmt.Sprintf(`"q_%s":{"noul":0.05},`, id)
			}
			return body + fmt.Sprintf(`"q_policy_violation":{"noul":%s},"q_policy_exception":{"noul":%s}}}`, violation, exception)
		}
		for name, tc := range map[string]struct {
			violation, exception string
			want                 Risk
		}{
			"violation floors at 7":       {"0.95", "0.05", 7},
			"exception cancels violation": {"0.95", "0.95", 0},
			"exception alone is nothing":  {"0.05", "0.95", 0},
		} {
			srv, _, _ := serveSystemOne(t, clean(tc.violation, tc.exception))
			d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), withRules(familyRequest(tool), testRules))
			if d.Risk != tc.want || d.Floor != tc.want {
				t.Errorf("%s %s: risk %d floor %d, want %d", fc.family, name, d.Risk, d.Floor, tc.want)
			}
		}
	}
}
