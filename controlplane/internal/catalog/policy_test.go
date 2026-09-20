package catalog

import (
	"path/filepath"
	"testing"
)

func TestOverlayPathPolicyDockerVolumesAndExtraPaths(t *testing.T) {
	overlay := DefaultOverlay(false)
	overlay.Volumes = append(overlay.Volumes, VolumeRow{
		ID:          "vol_cache",
		Name:        strPtr("cache"),
		Target:      strPtr("/cache"),
		Whitelisted: boolPtr(true),
		Read:        boolPtr(true),
		Write:       boolPtr(false),
		Exec:        boolPtr(false),
	}, VolumeRow{
		ID:          "vol_hidden",
		Name:        strPtr("hidden"),
		Target:      strPtr("/secret"),
		Whitelisted: boolPtr(false),
		Read:        boolPtr(true),
		Write:       boolPtr(true),
	})
	overlay.ExtraPaths = []PathRow{{
		ID:          "path_tmp",
		Path:        strPtr("/tmp"),
		Whitelisted: boolPtr(true),
		Read:        boolPtr(true),
		Write:       boolPtr(true),
		Exec:        boolPtr(false),
	}}
	got := OverlayPathPolicy(overlay, "docker", DefaultWorkspaceRoot, "")
	if len(got) != 3 {
		t.Fatalf("grants = %+v", got)
	}
	if got[0].Path != "/workspace" || !got[0].Read || !got[0].Write || !got[0].Exec {
		t.Fatalf("workspace grant = %+v", got[0])
	}
	if got[1].Path != "/cache" || !got[1].Read || got[1].Write {
		t.Fatalf("cache grant = %+v", got[1])
	}
	if got[2].Path != "/tmp" || !got[2].Write || got[2].Exec {
		t.Fatalf("extra grant = %+v", got[2])
	}
}

func TestOverlayPathPolicyLocalMapsWorkspaceVolume(t *testing.T) {
	host := filepath.FromSlash("/data/projects/proj_a/workspace")
	overlay := DefaultOverlay(false)
	overlay.Volumes = append(overlay.Volumes, VolumeRow{
		ID:          "vol_cache",
		Target:      strPtr("/cache"),
		Whitelisted: boolPtr(true),
		Read:        boolPtr(true),
		Write:       boolPtr(true),
	})
	overlay.ExtraPaths = []PathRow{{
		ID:          "path_tmp",
		Path:        strPtr("/tmp/extra"),
		Whitelisted: boolPtr(true),
		Read:        boolPtr(true),
		Write:       boolPtr(true),
	}}
	got := OverlayPathPolicy(overlay, "local", DefaultWorkspaceRoot, host)
	if len(got) != 2 {
		t.Fatalf("grants = %+v", got)
	}
	if got[0].Path != filepath.Clean(host) {
		t.Fatalf("mapped workspace = %q", got[0].Path)
	}
	if got[1].Path != filepath.Clean("/tmp/extra") {
		t.Fatalf("extra = %q", got[1].Path)
	}
}
