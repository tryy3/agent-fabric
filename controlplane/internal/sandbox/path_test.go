package sandbox_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestResolveUnderRootAcceptsRelative(t *testing.T) {
	root := t.TempDir()
	got, err := sandbox.ResolveUnderRoot(root, "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "a", "b.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveUnderRootRejectsDotDot(t *testing.T) {
	root := t.TempDir()
	_, err := sandbox.ResolveUnderRoot(root, "../outside.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveUnderRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "leak")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	_, err := sandbox.ResolveUnderRoot(root, "leak/secret.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("err = %v", err)
	}
}
