package fsops_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
	"github.com/tryy3/agent-fabric/internal/workspace/fsops"
)

func newFS(t *testing.T) sandbox.FS {
	t.Helper()
	env, err := local.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fsys, _ := env.FS()
	ctx := context.Background()
	for _, p := range []string{"a.txt", "dir/inner/b.txt", ".env", "notes.tar.gz"} {
		if err := fsys.WriteFile(ctx, p, []byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	return fsys
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"a.txt": "a.txt", "/a/b/": "a/b", "./a//b": "a/b", " a ": "a"} {
		got, err := fsops.Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", ".", "/", "a/.."} {
		if _, err := fsops.Normalize(in); !errors.Is(err, fsops.ErrInvalidPath) {
			t.Errorf("Normalize(%q) err = %v, want ErrInvalidPath", in, err)
		}
	}
	for _, in := range []string{"..", "../x", "a/../../x"} {
		if _, err := fsops.Normalize(in); !errors.Is(err, fsops.ErrEscapesRoot) {
			t.Errorf("Normalize(%q) err = %v, want ErrEscapesRoot", in, err)
		}
	}
}

func TestMoveValidation(t *testing.T) {
	ctx := context.Background()
	svc := fsops.New(newFS(t))
	cases := []struct {
		name     string
		from, to string
		want     error
	}{
		{"same path", "a.txt", "/a.txt", fsops.ErrInvalidPath},
		{"into itself", "dir", "dir/inner/dir", fsops.ErrIntoSelf},
		{"root source", ".", "x", fsops.ErrInvalidPath},
		{"escape", "a.txt", "../x", fsops.ErrEscapesRoot},
		{"missing source", "nope", "x", fsops.ErrNotFound},
		{"missing parent", "a.txt", "nope/a.txt", fsops.ErrNotFound},
		{"parent is file", "dir", "a.txt/dir", fsops.ErrInvalidPath},
		{"exists", "a.txt", "dir", fsops.ErrExists},
	}
	for _, tc := range cases {
		if _, err := svc.Move(ctx, tc.from, tc.to); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestMoveAndCopySucceed(t *testing.T) {
	ctx := context.Background()
	fsys := newFS(t)
	svc := fsops.New(fsys)
	got, err := svc.Move(ctx, "/dir", "renamed")
	if err != nil || got != "renamed" {
		t.Fatalf("Move = %q, %v", got, err)
	}
	if _, err := fsys.Stat(ctx, "renamed/inner/b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Copy(ctx, "a.txt", "renamed/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Stat(ctx, "a.txt"); err != nil {
		t.Fatal("copy removed the source")
	}
}

func TestDuplicateNaming(t *testing.T) {
	ctx := context.Background()
	fsys := newFS(t)
	svc := fsops.New(fsys)
	for _, step := range []struct{ from, want string }{
		{"a.txt", "a copy.txt"},
		{"a.txt", "a copy 2.txt"},
		{"a copy.txt", "a copy copy.txt"},
		{".env", ".env copy"},
		{"notes.tar.gz", "notes.tar copy.gz"},
		{"dir", "dir copy"},
		{"dir", "dir copy 2"},
	} {
		got, err := svc.Duplicate(ctx, step.from)
		if err != nil || got != step.want {
			t.Fatalf("Duplicate(%q) = %q, %v; want %q", step.from, got, err, step.want)
		}
	}
	if _, err := fsys.Stat(ctx, "dir copy 2/inner/b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Duplicate(ctx, "nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}

func TestDuplicateInNestedDir(t *testing.T) {
	svc := fsops.New(newFS(t))
	got, err := svc.Duplicate(context.Background(), "dir/inner/b.txt")
	if err != nil || got != "dir/inner/b copy.txt" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestGuardVetoesBeforeMutation(t *testing.T) {
	ctx := context.Background()
	fsys := newFS(t)
	var seen []string
	svc := fsops.New(fsys, fsops.WithGuard(func(_ context.Context, op fsops.Op, from, to string) error {
		seen = append(seen, string(op)+":"+from+"->"+to)
		if from == ".env" {
			return fsops.ErrProtected
		}
		return nil
	}))
	if _, err := svc.Move(ctx, ".env", "other"); !errors.Is(err, fsops.ErrProtected) {
		t.Fatalf("err = %v", err)
	}
	if _, err := fsys.Stat(ctx, ".env"); err != nil {
		t.Fatal("guard did not prevent the move")
	}
	if _, err := svc.Duplicate(ctx, ".env"); !errors.Is(err, fsops.ErrProtected) {
		t.Fatalf("duplicate err = %v", err)
	}
	if _, err := svc.Move(ctx, "a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen[2] != "move:a.txt->b.txt" {
		t.Fatalf("guard calls = %v", seen)
	}
}

func TestProtectGit(t *testing.T) {
	svc := fsops.New(newFS(t), fsops.WithGuard(fsops.ProtectGit))
	ctx := context.Background()
	for _, c := range [][2]string{{".git", "x"}, {"a.txt", ".git/a.txt"}} {
		if _, err := svc.Move(ctx, c[0], c[1]); !errors.Is(err, fsops.ErrProtected) {
			t.Fatalf("Move(%q,%q) err = %v, want ErrProtected", c[0], c[1], err)
		}
	}
}
