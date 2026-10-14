package catalog

import (
	"encoding/json"
	"testing"
)

// A settings patch that carries permissions.questions replaces the stored
// questions, so a reset override or deleted custom question does not survive.
func TestMergeSettingsReplacesPermissionQuestions(t *testing.T) {
	stored := json.RawMessage(`{"permissions":{"mode":"ask","questions":{"builtins":{"a":{"risk":9},"b":{"enabled":false}},"custom":[{"id":"x"}]}}}`)
	patch := json.RawMessage(`{"permissions":{"questions":{"builtins":{"b":{"enabled":false}}}}}`)
	out, err := MergeSettings(stored, patch)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"permissions":{"mode":"ask","questions":{"builtins":{"b":{"enabled":false}}}}}`
	if string(out) != want {
		t.Fatalf("got %s, want %s", out, want)
	}
}

// An assistant that sets only cascade options keeps the plane's scorer tiers.
func TestEffectivePermissionsTuningOnlyKeepsPlaneTiers(t *testing.T) {
	plane := Permissions{Scorers: &PermissionScorers{Fast: &PermissionScorer{ConnectionID: "c", Model: "m"}, InstructionsMaxChars: 12000}}
	got := EffectivePermissions(plane, Permissions{Scorers: &PermissionScorers{InstructionsMaxChars: 8000}})
	if got.Scorers == nil || got.Scorers.Fast == nil || got.Scorers.InstructionsMaxChars != 8000 {
		t.Fatalf("scorers = %+v", got.Scorers)
	}
	if plane.Scorers.InstructionsMaxChars != 12000 {
		t.Fatal("the plane's scorers were modified")
	}
}
