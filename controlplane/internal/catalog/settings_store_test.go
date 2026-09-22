package catalog_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func openGooseStore(t *testing.T) *catalog.Store {
	t.Helper()
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return catalog.Open(pool)
}

func TestEnsurePlaneSettingsSeedsFreshVolumeTemplate(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	settings, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{})
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := catalog.DecodeOverlay(settings.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Kind == nil || *overlay.Kind != catalog.DefaultSandboxKind {
		t.Fatalf("kind = %v", overlay.Kind)
	}
	if overlay.Image == nil || *overlay.Image != catalog.DefaultSandboxImage {
		t.Fatalf("image = %v", overlay.Image)
	}
	if len(overlay.Volumes) == 0 || overlay.Volumes[0].Name == nil || *overlay.Volumes[0].Name != catalog.DefaultVolumeNameTemplate {
		t.Fatalf("fresh volume template = %+v", overlay.Volumes)
	}
}

func TestEnsurePlaneSettingsPreservesPhase1VolumeOnUpgrade(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	if _, err := store.CreateProject(ctx, "Landing", ""); err != nil {
		t.Fatal(err)
	}
	settings, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{})
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := catalog.DecodeOverlay(settings.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Volumes[0].Name == nil || *overlay.Volumes[0].Name != catalog.Phase1VolumeNameTemplate {
		t.Fatalf("upgrade volume template = %v", overlay.Volumes[0].Name)
	}
}

func TestEnsurePlaneSettingsMigratesDeprecatedKeysOnce(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	first, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{
		Kind:  "local",
		Image: "golang:1.23",
	})
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := catalog.DecodeOverlay(first.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Kind == nil || *overlay.Kind != "local" || overlay.Image == nil || *overlay.Image != "golang:1.23" {
		t.Fatalf("migrated overlay = %+v", overlay)
	}

	second, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{
		Kind:  "docker",
		Image: "ignored:later",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := catalog.DecodeOverlay(second.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if again.Kind == nil || *again.Kind != "local" || again.Image == nil || *again.Image != "golang:1.23" {
		t.Fatalf("second seed mutated overlay = %+v", again)
	}
}

func TestPatchPlaneSettingsMergesScalarsAndLeavesSiblings(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	if _, err := store.GetPlaneSettings(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.PatchPlaneSettings(ctx, json.RawMessage(`{"image":"golang:1.23"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := catalog.DecodeOverlay(got.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Image == nil || *overlay.Image != "golang:1.23" {
		t.Fatalf("image = %v", overlay.Image)
	}
	if overlay.Kind == nil || *overlay.Kind != catalog.DefaultSandboxKind {
		t.Fatalf("kind should remain: %v", overlay.Kind)
	}
}

func TestUpdateAgentMergesSandboxSettings(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "m1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateAgent(ctx, ag.ID, nil, nil, nil, nil, json.RawMessage(`{"sandbox":{"image":"golang:1.23"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated.Settings), `"golang:1.23"`) {
		t.Fatalf("sandbox not merged: %s", updated.Settings)
	}
	again, err := store.UpdateAgent(ctx, ag.ID, nil, nil, nil, nil, json.RawMessage(`{"sandbox":{"kind":"local"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(again.Settings), `"golang:1.23"`) || !strings.Contains(string(again.Settings), `"local"`) {
		t.Fatalf("second sandbox patch replaced blob: %s", again.Settings)
	}
	withMCP, err := store.UpdateAgent(ctx, ag.ID, nil, nil, nil, nil, json.RawMessage(`{"mcp":{"servers":[]},"sandbox":{"idleTTLSeconds":600}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withMCP.Settings), `"golang:1.23"`) || !strings.Contains(string(withMCP.Settings), `"mcp"`) || !strings.Contains(string(withMCP.Settings), `"idleTTLSeconds"`) {
		t.Fatalf("nested settings clobbered sandbox: %s", withMCP.Settings)
	}
}
