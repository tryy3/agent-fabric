// Package fsconformance is a behavior suite every sandboxcore.FS must pass for
// the path operations (Rename, Copy) that the workspace service layer builds on.
package fsconformance

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

// Factory returns a fresh FS rooted at root, an empty directory on the host
// filesystem. Both live on the same machine so the suite can plant symlinks.
type Factory func(t *testing.T, root string) sandboxcore.FS

// RunRenameCopy runs the Rename and Copy conformance cases against newFS.
func RunRenameCopy(t *testing.T, newFS Factory) {
	t.Helper()
	ctx := context.Background()
	setup := func(t *testing.T) (sandboxcore.FS, string) {
		root := t.TempDir()
		fsys := newFS(t, root)
		must(t, fsys.WriteFile(ctx, "a.txt", []byte("alpha")))
		must(t, fsys.WriteFile(ctx, "dir/inner/b.txt", []byte("bravo")))
		must(t, fsys.Mkdir(ctx, "target"))
		return fsys, root
	}

	t.Run("rename file", func(t *testing.T) {
		fsys, _ := setup(t)
		must(t, fsys.Rename(ctx, "a.txt", "target/a2.txt"))
		wantContent(t, fsys, "target/a2.txt", "alpha")
		if _, err := fsys.Stat(ctx, "a.txt"); !errors.Is(err, fs.ErrNotExist) && err == nil {
			t.Fatal("source still exists")
		}
	})
	t.Run("rename directory carries descendants", func(t *testing.T) {
		fsys, _ := setup(t)
		must(t, fsys.Rename(ctx, "dir", "moved"))
		wantContent(t, fsys, "moved/inner/b.txt", "bravo")
	})
	t.Run("rename refuses existing file", func(t *testing.T) {
		fsys, _ := setup(t)
		must(t, fsys.WriteFile(ctx, "other.txt", []byte("keep")))
		err := fsys.Rename(ctx, "a.txt", "other.txt")
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("err = %v, want fs.ErrExist", err)
		}
		wantContent(t, fsys, "other.txt", "keep")
		wantContent(t, fsys, "a.txt", "alpha")
	})
	t.Run("rename refuses existing directory", func(t *testing.T) {
		fsys, _ := setup(t)
		if err := fsys.Rename(ctx, "dir", "target"); !errors.Is(err, fs.ErrExist) {
			t.Fatalf("err = %v, want fs.ErrExist", err)
		}
	})
	t.Run("rename missing source", func(t *testing.T) {
		fsys, _ := setup(t)
		if err := fsys.Rename(ctx, "nope.txt", "x.txt"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rename missing destination parent", func(t *testing.T) {
		fsys, _ := setup(t)
		if err := fsys.Rename(ctx, "a.txt", "missing/a.txt"); err == nil {
			t.Fatal("expected error")
		}
		wantContent(t, fsys, "a.txt", "alpha")
	})
	t.Run("rename rejects escapes", func(t *testing.T) {
		fsys, _ := setup(t)
		if err := fsys.Rename(ctx, "a.txt", "../outside.txt"); err == nil {
			t.Fatal("expected escape error")
		}
		if err := fsys.Rename(ctx, "../etc/passwd", "x.txt"); err == nil {
			t.Fatal("expected escape error")
		}
	})

	t.Run("copy file", func(t *testing.T) {
		fsys, _ := setup(t)
		must(t, fsys.Copy(ctx, "a.txt", "a copy.txt"))
		wantContent(t, fsys, "a copy.txt", "alpha")
		wantContent(t, fsys, "a.txt", "alpha")
	})
	t.Run("copy directory recursively", func(t *testing.T) {
		fsys, _ := setup(t)
		must(t, fsys.Copy(ctx, "dir", "dir copy"))
		wantContent(t, fsys, "dir copy/inner/b.txt", "bravo")
		wantContent(t, fsys, "dir/inner/b.txt", "bravo")
	})
	t.Run("copy refuses existing destination", func(t *testing.T) {
		fsys, _ := setup(t)
		if err := fsys.Copy(ctx, "a.txt", "target"); !errors.Is(err, fs.ErrExist) {
			t.Fatalf("err = %v, want fs.ErrExist", err)
		}
	})
	t.Run("copy rejects escapes", func(t *testing.T) {
		fsys, _ := setup(t)
		if err := fsys.Copy(ctx, "a.txt", "../outside.txt"); err == nil {
			t.Fatal("expected escape error")
		}
	})
	t.Run("copy refuses symlinks in the tree", func(t *testing.T) {
		fsys, root := setup(t)
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "dir", "link")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := fsys.Copy(ctx, "dir", "dir2"); err == nil {
			t.Fatal("expected symlink error")
		}
	})
}

func wantContent(t *testing.T, fsys sandboxcore.FS, p, want string) {
	t.Helper()
	got, err := fsys.ReadFile(context.Background(), p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", p, got, want)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
