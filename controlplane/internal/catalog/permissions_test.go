package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestDecodePermissions(t *testing.T) {
	ok := `{"mode":"auto","rules":[{"tool":"run_command","match":"git push *","action":"ask","risk":7}],
		"scorers":{"fast":{"connectionId":"c1","model":"jev-latest","strategy":"score"},
		           "deep":{"connectionId":"c2","model":"gemma"},"minConfidence":0.7,"maxLower":0,"skipAtOrBelow":2}}`
	p, err := catalog.DecodePermissions(json.RawMessage(ok))
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != "auto" || len(p.Rules) != 1 || p.Rules[0].Risk != 7 || p.Scorers.Fast.Model != "jev-latest" ||
		p.Scorers.MaxLower == nil || *p.Scorers.MaxLower != 0 {
		t.Fatalf("%+v", p)
	}
	for name, bad := range map[string]string{
		"unknown key":        `{"mood":"ask"}`,
		"unknown mode":       `{"mode":"yolo"}`,
		"rule without tool":  `{"rules":[{"match":"x","action":"ask"}]}`,
		"rule without match": `{"rules":[{"tool":"run_command","action":"ask"}]}`,
		"unknown action":     `{"rules":[{"tool":"run_command","match":"x","action":"maybe"}]}`,
		"risk out of range":  `{"rules":[{"tool":"run_command","match":"x","action":"ask","risk":11}]}`,
		"unknown rule key":   `{"rules":[{"tool":"run_command","match":"x","action":"ask","when":"now"}]}`,
		"scorer no model":    `{"scorers":{"deep":{"connectionId":"c"}}}`,
		"scorer no conn":     `{"scorers":{"fast":{"model":"m"}}}`,
		"unknown strategy":   `{"scorers":{"fast":{"connectionId":"c","model":"m","strategy":"vibes"}}}`,
		"deep strategy":      `{"scorers":{"deep":{"connectionId":"c","model":"m","strategy":"score"}}}`,
		"confidence range":   `{"scorers":{"minConfidence":1.5}}`,
		"maxLower range":     `{"scorers":{"maxLower":10}}`,
		"not an object":      `[]`,
	} {
		if _, err := catalog.DecodePermissions(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: accepted %s", name, bad)
		}
	}
}

func TestEffectivePermissions(t *testing.T) {
	plane := catalog.Permissions{
		Rules:   []catalog.PermissionRule{{Tool: "run_command", Match: "rm *", Action: "deny"}},
		Scorers: &catalog.PermissionScorers{Deep: &catalog.PermissionScorer{ConnectionID: "plane", Model: "m"}},
	}
	own := catalog.Permissions{Mode: "auto", Rules: []catalog.PermissionRule{{Tool: "run_command", Match: "ls", Action: "allow"}}}
	eff := catalog.EffectivePermissions(plane, own)
	if eff.Mode != "auto" || len(eff.Rules) != 2 || eff.Rules[0].Action != "deny" || eff.Scorers.Deep.ConnectionID != "plane" {
		t.Fatalf("%+v", eff)
	}
	own.Scorers = &catalog.PermissionScorers{Fast: &catalog.PermissionScorer{ConnectionID: "own", Model: "jev"}}
	if eff = catalog.EffectivePermissions(plane, own); eff.Scorers.Fast == nil || eff.Scorers.Deep != nil {
		t.Fatalf("the assistant's scorers replace the plane's: %+v", eff.Scorers)
	}
	if len(plane.Rules) != 1 {
		t.Fatal("inputs must not be modified")
	}
}

func TestPatchPlanePermissions(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}
	patched, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
		Permissions: json.RawMessage(`{"rules":[{"tool":"*","match":"**/.env*","action":"deny"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched.Permissions), ".env") {
		t.Fatalf("patch result = %s", patched.Permissions)
	}
	got, err := store.GetPlaneSettings(ctx)
	if err != nil || !strings.Contains(string(got.Permissions), ".env") {
		t.Fatalf("stored = %s, %v", got.Permissions, err)
	}
	perms, err := store.PlanePermissions(ctx)
	if err != nil || len(perms.Rules) != 1 || perms.Rules[0].Action != "deny" {
		t.Fatalf("%+v, %v", perms, err)
	}
	for _, bad := range []string{`{"mode":"full"}`, `{"rules":[{"tool":"x"}]}`} {
		if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Permissions: json.RawMessage(bad)}); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	// null clears; other settings are untouched by a permissions patch.
	cleared, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Permissions: json.RawMessage(`null`)})
	if err != nil || string(cleared.Permissions) != `{}` || string(cleared.Sandbox) != string(got.Sandbox) {
		t.Fatalf("cleared = %s sandbox %s, %v", cleared.Permissions, cleared.Sandbox, err)
	}
}

func TestAssistantPermissionsPatchValidates(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	conn, err := store.CreateInferenceConnection(ctx, "c", catalog.TypeOpenAICompatible, "http://127.0.0.1:1/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceInferenceConnectionModels(ctx, conn.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAssistant(ctx, "A", "", "", conn.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	good := json.RawMessage(`{"permissions":{"rules":[{"tool":"run_command","match":"make *","action":"allow"}]}}`)
	updated, err := store.UpdateAssistant(ctx, ag.ID, nil, nil, nil, nil, nil, good)
	if err != nil {
		t.Fatal(err)
	}
	p, err := catalog.PermissionsFromSettings(updated.Settings)
	if err != nil || len(p.Rules) != 1 {
		t.Fatalf("%+v, %v", p, err)
	}
	bad := json.RawMessage(`{"permissions":{"rules":[{"tool":"run_command","match":"make *","action":"sometimes"}]}}`)
	if _, err := store.UpdateAssistant(ctx, ag.ID, nil, nil, nil, nil, nil, bad); err == nil {
		t.Fatal("accepted an unknown action")
	}
}

func TestDecodePermissionsBuiltinsAndDeepOptions(t *testing.T) {
	prev := catalog.ValidatePermissionBuiltinsFunc
	t.Cleanup(func() { catalog.ValidatePermissionBuiltinsFunc = prev })
	catalog.ValidatePermissionBuiltinsFunc = func(in map[string]catalog.PermissionBuiltin) error {
		for id := range in {
			if id != "command.forbidden" {
				return fmt.Errorf("unknown built-in tier %q", id)
			}
		}
		return nil
	}
	p, err := catalog.DecodePermissions(json.RawMessage(`{
		"builtins":{"command.forbidden":{"add":["terraform"],"remove":["docker"],"risk":10,"consult":false}},
		"scorers":{"deep":{"connectionId":"c","model":"m","style":"bands","enableThinking":false,"maxTokens":256,"structuredOutput":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	b := p.Builtins["command.forbidden"]
	if len(b.Add) != 1 || b.Consult == nil || *b.Consult || !p.Scorers.Deep.StructuredOutput || *p.Scorers.Deep.EnableThinking {
		t.Fatalf("%+v %+v", b, p.Scorers.Deep)
	}
	for name, bad := range map[string]string{
		"unknown tier":      `{"builtins":{"command.nope":{"risk":3}}}`,
		"risk range":        `{"builtins":{"command.forbidden":{"risk":0,"add":[]},"x":{"risk":12}}}`,
		"unknown style":     `{"scorers":{"deep":{"connectionId":"c","model":"m","style":"vibes"}}}`,
		"fast deep options": `{"scorers":{"fast":{"connectionId":"c","model":"m","structuredOutput":true}}}`,
		"max tokens":        `{"scorers":{"deep":{"connectionId":"c","model":"m","maxTokens":-1}}}`,
	} {
		if _, err := catalog.DecodePermissions(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: accepted %s", name, bad)
		}
	}
	eff := catalog.EffectivePermissions(
		catalog.Permissions{Builtins: map[string]catalog.PermissionBuiltin{"a": {Risk: 3}, "b": {Risk: 4}}},
		catalog.Permissions{Builtins: map[string]catalog.PermissionBuiltin{"b": {Risk: 6}}},
	)
	if eff.Builtins["a"].Risk != 3 || eff.Builtins["b"].Risk != 6 {
		t.Fatalf("the assistant's override wins per tier: %+v", eff.Builtins)
	}
}
