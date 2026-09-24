package local_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
)

func TestLocalFSReadWriteAndJail(t *testing.T) {
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	fsys, ok := env.FS()
	if !ok {
		t.Fatal("expected FS")
	}
	ctx := context.Background()
	if err := fsys.WriteFile(ctx, "hello.txt", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	b, err := fsys.ReadFile(ctx, "hello.txt")
	if err != nil || string(b) != "hi" {
		t.Fatalf("read = %q err=%v", b, err)
	}
	if _, err := fsys.ReadFile(ctx, "../nope.txt"); err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("jail err = %v", err)
	}
	if filepath.Base(env.ID()) == "" {
		t.Fatal("empty id")
	}
}

func TestLocalFSReadDirMkdirRemoveAndJail(t *testing.T) {
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	fsys, ok := env.FS()
	if !ok {
		t.Fatal("expected FS")
	}
	ctx := context.Background()
	if err := fsys.Mkdir(ctx, "src"); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(ctx, "src/index.html", []byte("<h1>hi</h1>")); err != nil {
		t.Fatal(err)
	}
	entries, err := fsys.ReadDir(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "src" || !entries[0].IsDir {
		t.Fatalf("root entries = %+v", entries)
	}
	nested, err := fsys.ReadDir(ctx, "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(nested) != 1 || nested[0].Name != "index.html" || nested[0].IsDir {
		t.Fatalf("src entries = %+v", nested)
	}
	if _, err := fsys.ReadDir(ctx, "../nope"); err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("jail ReadDir err = %v", err)
	}
	if err := fsys.Mkdir(ctx, "../outside"); err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("jail Mkdir err = %v", err)
	}
	if err := fsys.Remove(ctx, "../nope"); err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("jail Remove err = %v", err)
	}
	if err := fsys.Remove(ctx, "src"); err == nil {
		t.Fatal("expected non-empty directory remove to fail")
	}
	if err := fsys.Remove(ctx, "src/index.html"); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Remove(ctx, "src"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalExecutorRunsUnderWorkspace(t *testing.T) {
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	executor, ok := env.Exec()
	if !ok {
		t.Fatal("expected Executor")
	}

	result, err := executor.Run(context.Background(), sandbox.ExecRequest{
		Cmd:     []string{"pwd"},
		WorkDir: "nested",
	})
	if err == nil {
		t.Fatal("expected missing work directory error")
	}

	if err := envMustWrite(t, env, "nested/file.txt", "hello"); err != nil {
		t.Fatal(err)
	}
	result, err = executor.Run(context.Background(), sandbox.ExecRequest{
		Cmd:     []string{"pwd"},
		WorkDir: "nested",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(result.Stdout)) != filepath.Join(root, "nested") {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestLocalExecutorReturnsTimeoutError(t *testing.T) {
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	executor, ok := env.Exec()
	if !ok {
		t.Fatal("expected Executor")
	}

	_, err = executor.Run(context.Background(), sandbox.ExecRequest{
		Cmd:     []string{"sleep", "10"},
		Timeout: 10 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline exceeded", err)
	}
}

func envMustWrite(t *testing.T, env sandbox.Environment, path, data string) error {
	t.Helper()
	fsys, ok := env.FS()
	if !ok {
		t.Fatal("expected FS")
	}
	return fsys.WriteFile(context.Background(), path, []byte(data))
}
