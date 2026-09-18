package sandboxcore

import "testing"

func TestContainUnderRootPOSIXAcceptsAbsoluteUnderRoot(t *testing.T) {
	got, err := ContainUnderRootPOSIX("/workspace", "/workspace/root.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/root.json" {
		t.Fatalf("got %q", got)
	}
}

func TestContainUnderRootPOSIXAcceptsRootItself(t *testing.T) {
	got, err := ContainUnderRootPOSIX("/workspace", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace" {
		t.Fatalf("got %q", got)
	}
}

func TestContainUnderRootPOSIXAcceptsRelative(t *testing.T) {
	got, err := ContainUnderRootPOSIX("/workspace", "./root.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/root.json" {
		t.Fatalf("got %q", got)
	}
}

func TestContainUnderRootPOSIXAcceptsEmptyAsRoot(t *testing.T) {
	got, err := ContainUnderRootPOSIX("/workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace" {
		t.Fatalf("got %q", got)
	}
}

func TestContainUnderRootPOSIXRejectsOutsideAbsolute(t *testing.T) {
	_, err := ContainUnderRootPOSIX("/workspace", "/not-workdir")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestContainUnderRootPOSIXRejectsDotDotEscape(t *testing.T) {
	_, err := ContainUnderRootPOSIX("/workspace", "../root.json")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestContainUnderRootPOSIXRejectsNestedDotDotEscape(t *testing.T) {
	_, err := ContainUnderRootPOSIX("/workspace", "a/../../outside")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestContainUnderRootPOSIXRejectsPrefixSibling(t *testing.T) {
	_, err := ContainUnderRootPOSIX("/workspace", "/workspace-other/x")
	if err == nil {
		t.Fatal("expected error")
	}
}
