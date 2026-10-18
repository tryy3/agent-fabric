package catalog_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func ip(v int) *int { return &v }

func TestResolveLimitsNormalPrompt(t *testing.T) {
	cases := []struct {
		name      string
		plane     catalog.Limits
		assistant catalog.Limits
		want      int
	}{
		{"built-in default", catalog.Limits{}, catalog.Limits{}, 8},
		{"plane default", catalog.Limits{DefaultToolRounds: ip(12)}, catalog.Limits{}, 12},
		{"assistant raises within the plane max", catalog.Limits{MaxToolRounds: ip(50)}, catalog.Limits{DefaultToolRounds: ip(30)}, 30},
		{"assistant cannot pass the plane max", catalog.Limits{MaxToolRounds: ip(20)}, catalog.Limits{DefaultToolRounds: ip(30)}, 20},
		{"assistant lowers", catalog.Limits{}, catalog.Limits{MaxToolRounds: ip(5)}, 5},
	}
	for _, c := range cases {
		if got := catalog.ResolveLimits(c.plane, c.assistant, catalog.RunRequest{}).ToolRounds; got != c.want {
			t.Errorf("%s: rounds = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestResolveLimitsSkillRun(t *testing.T) {
	skill := func(ceiling int, depth string, seconds int) catalog.RunRequest {
		return catalog.RunRequest{Skill: true, SkillMaxToolRounds: ceiling, Depth: depth, Seconds: seconds}
	}
	if got := catalog.ResolveLimits(catalog.Limits{}, catalog.Limits{}, skill(80, "", 0)).ToolRounds; got != 80 {
		t.Errorf("skill ceiling = %d, want 80", got)
	}
	if got := catalog.ResolveLimits(catalog.Limits{}, catalog.Limits{}, skill(0, "", 0)).ToolRounds; got != 100 {
		t.Errorf("no skill ceiling = %d, want plane max 100", got)
	}
	if got := catalog.ResolveLimits(catalog.Limits{MaxToolRounds: ip(40)}, catalog.Limits{}, skill(80, "", 0)).ToolRounds; got != 40 {
		t.Errorf("plane max wins = %d, want 40", got)
	}
	if got := catalog.ResolveLimits(catalog.Limits{}, catalog.Limits{MaxToolRounds: ip(30)}, skill(80, "", 0)).ToolRounds; got != 30 {
		t.Errorf("assistant limit wins = %d, want 30", got)
	}
	for depth, want := range map[string]int{"quick": 20, "normal": 48, "deep": 80, "": 80, "bogus": 80} {
		if got := catalog.ResolveLimits(catalog.Limits{}, catalog.Limits{}, skill(80, depth, 0)).ToolRounds; got != want {
			t.Errorf("depth %q = %d, want %d", depth, got, want)
		}
	}
	if got := catalog.ResolveLimits(catalog.Limits{}, catalog.Limits{}, skill(3, "quick", 0)).ToolRounds; got != 1 {
		t.Errorf("tiny ceiling with quick = %d, want at least 1", got)
	}
}

func TestResolveLimitsWallClock(t *testing.T) {
	plane := catalog.Limits{MaxTurnSeconds: ip(600)}
	lowered := catalog.ResolveLimits(plane, catalog.Limits{}, catalog.RunRequest{Skill: true, Seconds: 120})
	if lowered.Seconds != 120 || len(lowered.Notes) != 0 || lowered.Timeout() != 2*time.Minute {
		t.Errorf("lowered = %+v", lowered)
	}
	clamped := catalog.ResolveLimits(plane, catalog.Limits{}, catalog.RunRequest{Skill: true, Seconds: 7200})
	if clamped.Seconds != 600 || len(clamped.Notes) != 1 {
		t.Errorf("clamped = %+v", clamped)
	}
	if def := catalog.ResolveLimits(catalog.Limits{}, catalog.Limits{}, catalog.RunRequest{}); def.Seconds != 900 {
		t.Errorf("default seconds = %d, want 900", def.Seconds)
	}
}

func TestDecodeLimitsValidation(t *testing.T) {
	for _, bad := range []string{
		`{"maxToolRounds":0}`, `{"maxToolRounds":5000}`, `{"maxTurnSeconds":3}`,
		`{"defaultToolRounds":20,"maxToolRounds":10}`, `{"unknown":1}`, `{"maxToolRounds":"x"}`,
	} {
		if _, err := catalog.DecodeLimits(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
	got, err := catalog.DecodeLimits(json.RawMessage(`{"defaultToolRounds":8,"maxToolRounds":100,"maxTurnSeconds":900}`))
	if err != nil || *got.MaxToolRounds != 100 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestPlaneAndAssistantLimitsPersist(t *testing.T) {
	store := openGooseStore(t)
	ctx := context.Background()

	got, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Limits: json.RawMessage(`{"maxToolRounds":60,"maxTurnSeconds":300}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Limits) != `{"maxToolRounds":60,"maxTurnSeconds":300}` {
		t.Fatalf("limits = %s", got.Limits)
	}
	if l := store.PlaneLimits(ctx); l.MaxToolRounds == nil || *l.MaxToolRounds != 60 {
		t.Fatalf("PlaneLimits = %+v", l)
	}
	if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Limits: json.RawMessage(`{"maxToolRounds":0}`)}); err == nil {
		t.Fatal("expected invalid limits to be rejected")
	}
	cleared, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Limits: json.RawMessage(`null`)})
	if err != nil || string(cleared.Limits) != `{}` {
		t.Fatalf("cleared = %s, %v", cleared.Limits, err)
	}

	merged, err := catalog.MergeSettings(json.RawMessage(`{}`), json.RawMessage(`{"limits":{"maxToolRounds":30}}`))
	if err != nil || catalog.LimitsFromSettings(merged).MaxToolRounds == nil {
		t.Fatalf("merged = %s, %v", merged, err)
	}
	if _, err := catalog.MergeSettings(json.RawMessage(`{}`), json.RawMessage(`{"limits":{"maxToolRounds":-1}}`)); err == nil {
		t.Fatal("assistant limits must be validated")
	}
	if _, err := catalog.MergeSettings(json.RawMessage(`{}`), json.RawMessage(`{"skills":{"disabled":["Bad Name"]}}`)); err == nil {
		t.Fatal("skills toggles must be validated")
	}
}
