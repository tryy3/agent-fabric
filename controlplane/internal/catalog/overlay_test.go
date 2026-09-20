package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPatchOverlayReplacesScalarsAndDeletesNulls(t *testing.T) {
	base, err := EncodeOverlay(DefaultOverlay(true))
	if err != nil {
		t.Fatal(err)
	}
	patched, err := PatchOverlayJSON(base, json.RawMessage(`{"image":"golang:1.23","kind":null}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeOverlay(patched)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != nil {
		t.Fatalf("kind should be deleted, got %v", *got.Kind)
	}
	if got.Image == nil || *got.Image != "golang:1.23" {
		t.Fatalf("image = %v", got.Image)
	}
	if got.WorkspaceRoot == nil || *got.WorkspaceRoot != DefaultWorkspaceRoot {
		t.Fatalf("workspaceRoot = %v", got.WorkspaceRoot)
	}
}

func TestPatchOverlayMergesVolumesByID(t *testing.T) {
	base, err := EncodeOverlay(DefaultOverlay(true))
	if err != nil {
		t.Fatal(err)
	}
	patched, err := PatchOverlayJSON(base, json.RawMessage(`{
		"volumes": [
			{"id":"vol_workspace","name":"custom-vol"},
			{"id":"vol_cache","name":"cache","target":"/cache","enabled":true}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeOverlay(patched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Volumes) != 2 {
		t.Fatalf("volumes = %+v", got.Volumes)
	}
	if got.Volumes[0].ID != WorkspaceVolumeID || got.Volumes[0].Name == nil || *got.Volumes[0].Name != "custom-vol" {
		t.Fatalf("workspace volume = %+v", got.Volumes[0])
	}
	if got.Volumes[0].Target == nil || *got.Volumes[0].Target != DefaultWorkspaceRoot {
		t.Fatalf("workspace target should be inherited: %+v", got.Volumes[0])
	}
	if got.Volumes[1].ID != "vol_cache" {
		t.Fatalf("extra volume = %+v", got.Volumes[1])
	}
}

func TestResolveOverlayDenyWinsAndTombstone(t *testing.T) {
	global := DefaultOverlay(false)
	write := false
	project := Overlay{
		Image: strPtr("golang:1.23"),
		Volumes: []VolumeRow{{
			ID:    WorkspaceVolumeID,
			Write: &write,
		}, {
			ID:      "vol_tmp",
			Enabled: boolPtr(false),
			Name:    strPtr("tmp"),
		}},
	}
	agentWrite := true
	agent := Overlay{
		Volumes: []VolumeRow{{
			ID:    WorkspaceVolumeID,
			Write: &agentWrite,
		}},
	}
	got := ResolveOverlay(global, project, agent)
	if got.Image == nil || *got.Image != "golang:1.23" {
		t.Fatalf("image = %v", got.Image)
	}
	if len(got.Volumes) != 1 || got.Volumes[0].ID != WorkspaceVolumeID {
		t.Fatalf("volumes = %+v", got.Volumes)
	}
	if got.Volumes[0].Write == nil || *got.Volumes[0].Write {
		t.Fatalf("write should stay false: %+v", got.Volumes[0])
	}
}

func TestMergeSettingsSandboxLeavesSiblingKeys(t *testing.T) {
	settings := json.RawMessage(`{"sandbox":{"image":"alpine:3.20"},"allowedAgents":["agent_1"]}`)
	got, err := MergeSettingsSandbox(settings, json.RawMessage(`{"image":"golang:1.23"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"allowedAgents"`) {
		t.Fatalf("lost sibling keys: %s", got)
	}
	if !strings.Contains(string(got), `"golang:1.23"`) {
		t.Fatalf("sandbox not patched: %s", got)
	}
}

func TestPatchOverlayNullDeletesKeyedArrays(t *testing.T) {
	base, err := EncodeOverlay(DefaultOverlay(false))
	if err != nil {
		t.Fatal(err)
	}
	patched, err := PatchOverlayJSON(base, json.RawMessage(`{"volumes":null}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeOverlay(patched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Volumes) != 0 {
		t.Fatalf("volumes should be deleted: %+v", got.Volumes)
	}
}

func TestDefaultOverlayPreservesPhase1VolumeName(t *testing.T) {
	got := DefaultOverlay(true)
	if got.Volumes[0].Name == nil || *got.Volumes[0].Name != Phase1VolumeNameTemplate {
		t.Fatalf("upgrade volume name = %v", got.Volumes[0].Name)
	}
	fresh := DefaultOverlay(false)
	if fresh.Volumes[0].Name == nil || *fresh.Volumes[0].Name != DefaultVolumeNameTemplate {
		t.Fatalf("fresh volume name = %v", fresh.Volumes[0].Name)
	}
}

func strPtr(v string) *string { return &v }
func boolPtr(v bool) *bool    { return &v }
