package catalog_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestPlaneSkillsPatchNormalizesAndPersists(t *testing.T) {
	store := openGooseStore(t)
	ctx := context.Background()

	got, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
		Skills: json.RawMessage(`{"disabled":["zeta"," alpha ","zeta"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Skills) != `{"disabled":["alpha","zeta"]}` {
		t.Fatalf("skills = %s", got.Skills)
	}
	if d := store.PlaneSkills(ctx).Disabled; len(d) != 2 || d[0] != "alpha" || d[1] != "zeta" {
		t.Fatalf("PlaneSkills = %v", d)
	}

	if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
		Skills: json.RawMessage(`{"disabled":["Not A Name"]}`),
	}); err == nil {
		t.Fatal("expected invalid skill name to be rejected")
	}
	if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
		Skills: json.RawMessage(`{"enabled":["x"]}`),
	}); err == nil {
		t.Fatal("expected unknown key to be rejected")
	}

	cleared, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Skills: json.RawMessage(`null`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(cleared.Skills) != `{}` || len(store.PlaneSkills(ctx).Disabled) != 0 {
		t.Fatalf("cleared = %s", cleared.Skills)
	}
}

func TestSkillsFromSettingsIsLenient(t *testing.T) {
	cases := map[string]int{
		``:                              0,
		`null`:                          0,
		`{}`:                            0,
		`{"skills":{"disabled":["a"]}}`: 1,
		`{"skills":"junk"}`:             0,
		`not json`:                      0,
	}
	for raw, want := range cases {
		if got := len(catalog.SkillsFromSettings(json.RawMessage(raw)).Disabled); got != want {
			t.Errorf("%q: got %d disabled, want %d", raw, got, want)
		}
	}
}
