package askuser_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/tools/askuser"
)

func TestParseAndValidate(t *testing.T) {
	raw := json.RawMessage(`{
		"questions":[
			{"id":"a","question":"Q1?","options":[{"label":"One"},{"label":"Two"}]}
		]
	}`)
	args, err := askuser.ParseAndValidate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(args.Questions) != 1 || args.Questions[0].ID != "a" {
		t.Fatalf("%+v", args)
	}
}

func TestParseAndValidateRejectsBadCounts(t *testing.T) {
	_, err := askuser.ParseAndValidate(json.RawMessage(`{"questions":[]}`))
	if err == nil || !strings.Contains(err.Error(), "1–3") {
		t.Fatalf("err = %v", err)
	}
}
