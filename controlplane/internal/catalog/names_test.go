package catalog

import (
	"strings"
	"testing"
)

func TestExpandNameProjectAndThread(t *testing.T) {
	got, err := ExpandName("agent-fabric-container-{projectID}", NameVars{ProjectID: "proj_abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "agent-fabric-container-proj_abc" {
		t.Fatalf("got %q", got)
	}

	got, err = ExpandName("box-{threadID}", NameVars{ThreadID: "th_1"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "box-th_1" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandNameStaticUnchanged(t *testing.T) {
	got, err := ExpandName("shared-build-box", NameVars{ProjectID: "proj_a"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "shared-build-box" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandNameMissingProjectOrThread(t *testing.T) {
	if _, err := ExpandName("c-{projectID}", NameVars{}); err == nil || !strings.Contains(err.Error(), "projectID") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ExpandName("c-{threadID}", NameVars{ProjectID: "proj_a"}); err == nil || !strings.Contains(err.Error(), "threadID") {
		t.Fatalf("err = %v", err)
	}
}

func TestExpandNameRejectsUserID(t *testing.T) {
	if _, err := ExpandName("box-{userID}", NameVars{ProjectID: "proj_a"}); err == nil || !strings.Contains(err.Error(), "{userID}") {
		t.Fatalf("err = %v", err)
	}
}

func TestExpandNameUnknownVariable(t *testing.T) {
	if _, err := ExpandName("box-{envID}", NameVars{}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestExpandNameRandomIsEightHexAndUnique(t *testing.T) {
	first, err := ExpandName("ephemeral-{random}", NameVars{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExpandName("ephemeral-{random}", NameVars{})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("random reused: %q", first)
	}
	suffix := strings.TrimPrefix(first, "ephemeral-")
	if len(suffix) != 8 {
		t.Fatalf("random length = %d (%q)", len(suffix), suffix)
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Fatalf("random not hex: %q", suffix)
		}
	}
}

func TestApplyIdentityPrefix(t *testing.T) {
	if got := ApplyIdentityPrefix("shared-box", "dev-"); got != "dev-shared-box" {
		t.Fatalf("got %q", got)
	}
	if got := ApplyIdentityPrefix("dev-shared-box", "dev-"); got != "dev-shared-box" {
		t.Fatalf("already prefixed: %q", got)
	}
	if got := ApplyIdentityPrefix("shared-box", ""); got != "shared-box" {
		t.Fatalf("empty prefix: %q", got)
	}
}

func TestExpandVolumesTemplatesAndReadonly(t *testing.T) {
	write := false
	got, err := ExpandVolumes([]VolumeRow{
		{
			ID:     WorkspaceVolumeID,
			Name:   strPtr("shared-files"),
			Target: strPtr("/workspace"),
		},
		{
			ID:     "vol_cache",
			Name:   strPtr("cache-{projectID}"),
			Target: strPtr("/cache"),
			Write:  &write,
		},
		{
			ID:      "vol_skip",
			Name:    strPtr("incomplete"),
			Enabled: boolPtr(true),
		},
		{
			ID:      "vol_off",
			Enabled: boolPtr(false),
			Name:    strPtr("off"),
			Target:  strPtr("/off"),
		},
	}, NameVars{ProjectID: "proj_abc"}, "dev-")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("volumes = %+v", got)
	}
	if got[0].Name != "dev-shared-files" || got[0].Target != "/workspace" || got[0].ReadOnly {
		t.Fatalf("workspace = %+v", got[0])
	}
	if got[1].Name != "dev-cache-proj_abc" || got[1].Target != "/cache" || !got[1].ReadOnly {
		t.Fatalf("cache = %+v", got[1])
	}
	if !HasVolumeTarget(got, "/workspace") || HasVolumeTarget(got, "/missing") {
		t.Fatalf("target lookup = %+v", got)
	}
}

func TestExpandVolumesStaticNameSharedAcrossProjects(t *testing.T) {
	row := VolumeRow{ID: WorkspaceVolumeID, Name: strPtr("shared-files"), Target: strPtr("/workspace")}
	a, err := ExpandVolumes([]VolumeRow{row}, NameVars{ProjectID: "proj_a"}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExpandVolumes([]VolumeRow{row}, NameVars{ProjectID: "proj_b"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if a[0].Name != "shared-files" || b[0].Name != "shared-files" {
		t.Fatalf("names = %q %q", a[0].Name, b[0].Name)
	}
}

func TestExpandNameDifferentProjectIDsDiffer(t *testing.T) {
	a, err := ExpandName(DefaultContainerNameTemplate, NameVars{ProjectID: "proj_a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExpandName(DefaultContainerNameTemplate, NameVars{ProjectID: "proj_b"})
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("project templates collided: %q", a)
	}
}

func TestPreviewOverlayRendersRandomPlaceholder(t *testing.T) {
	name := "box-{random}"
	vol := "disk-{random}"
	got, err := PreviewOverlay(Overlay{
		ContainerName: &name,
		Volumes:       []VolumeRow{{ID: WorkspaceVolumeID, Name: &vol}},
	}, NameVars{ProjectID: "proj_a"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ContainerName == nil || *got.ContainerName != "box-<random>" {
		t.Fatalf("container = %v", got.ContainerName)
	}
	if got.Volumes[0].Name == nil || *got.Volumes[0].Name != "disk-<random>" {
		t.Fatalf("volume = %v", got.Volumes[0].Name)
	}
}
