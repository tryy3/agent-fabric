package catalog_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestEffectiveQuestionsMergePerID(t *testing.T) {
	off := false
	plane := catalog.Permissions{Questions: &catalog.PermissionQuestions{
		Builtins: map[string]catalog.PermissionQuestionOverride{"data_leak": {Risk: 9}, "destructive": {Text: "plane wording here"}},
		Custom: []catalog.PermissionCustomQuestion{
			{ID: "prod", Tools: []string{"command"}, Text: "plane prod question", Risk: 8},
			{ID: "plane_only", Tools: []string{"web"}, Text: "plane only question", Risk: 7},
		},
	}}
	assistant := catalog.Permissions{Questions: &catalog.PermissionQuestions{
		Builtins: map[string]catalog.PermissionQuestionOverride{"destructive": {Enabled: &off}},
		Custom:   []catalog.PermissionCustomQuestion{{ID: "prod", Tools: []string{"command"}, Text: "assistant prod question", Risk: 9}},
	}}
	q := catalog.EffectivePermissions(plane, assistant).Questions
	if q == nil || q.Builtins["data_leak"].Risk != 9 || q.Builtins["destructive"].Text != "" || q.Builtins["destructive"].Enabled == nil {
		t.Fatalf("the assistant's override replaces the plane's per id, the plane's others stay: %+v", q)
	}
	var ids []string
	for _, c := range q.Custom {
		ids = append(ids, c.ID+"="+c.Text)
	}
	if strings.Join(ids, ",") != "plane_only=plane only question,prod=assistant prod question" {
		t.Fatalf("custom questions = %v", ids)
	}
	if catalog.EffectivePermissions(catalog.Permissions{}, catalog.Permissions{}).Questions != nil {
		t.Error("no settings anywhere means no questions object")
	}
	if q := catalog.EffectivePermissions(plane, catalog.Permissions{}).Questions; q.Builtins["destructive"].Text != "plane wording here" || len(q.Custom) != 2 {
		t.Errorf("the plane default applies when the assistant sets nothing: %+v", q)
	}
}

func TestDecodePermissionsQuestions(t *testing.T) {
	prev := catalog.ValidatePermissionQuestionsFunc
	t.Cleanup(func() { catalog.ValidatePermissionQuestionsFunc = prev })
	catalog.ValidatePermissionQuestionsFunc = func(in catalog.PermissionQuestions) error {
		for id := range in.Builtins {
			if id != "data_leak" {
				return fmt.Errorf("unknown gate question %q", id)
			}
		}
		return nil
	}
	p, err := catalog.DecodePermissions(json.RawMessage(`{"questions":{"builtins":{"data_leak":{"enabled":false,"risk":9,"text":"Does it leak anything?"}},
		"custom":[{"id":"prod","tools":["command"],"kind":"tripwire","text":"Does it touch prod?","risk":9,"enabled":true}]}}`))
	if err != nil || *p.Questions.Builtins["data_leak"].Enabled || p.Questions.Custom[0].Tools[0] != "command" {
		t.Fatalf("%+v, %v", p.Questions, err)
	}
	for _, bad := range []string{
		`{"questions":{"builtins":{"nope":{"risk":3}}}}`,
		`{"questions":{"builtins":{"data_leak":{"kind":"policy"}}}}`,
		`{"questions":[]}`,
	} {
		if _, err := catalog.DecodePermissions(json.RawMessage(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
