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
