package execfs_test

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/execfs"
)

const (
	readScript  = `cat -- "$1"`
	writeScript = `set -eu
dir=$1
dst=$2
mkdir -p -- "$dir"
tmp=$(mktemp "$dir/.execfs.XXXXXX")
trap 'rm -f -- "$tmp"' EXIT HUP INT TERM
cat > "$tmp"
mv -f -- "$tmp" "$dst"
trap - EXIT HUP INT TERM`
	statScript = `stat -c '%s	%Y	%f' -- "$1"`
)

type fakeExec struct {
	run   func(sandbox.ExecRequest) (sandbox.ExecResult, error)
	calls []sandbox.ExecRequest
}

func (f *fakeExec) Run(_ context.Context, req sandbox.ExecRequest) (sandbox.ExecResult, error) {
	f.calls = append(f.calls, req)
	return f.run(req)
}

func TestFS_ReadFile(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{run: func(req sandbox.ExecRequest) (sandbox.ExecResult, error) {
		wantCmd := []string{"sh", "-c", readScript, "execfs", "/workspace/docs/a file.txt"}
		if !reflect.DeepEqual(req.Cmd, wantCmd) {
			t.Fatalf("command = %#v, want %#v", req.Cmd, wantCmd)
		}
		return sandbox.ExecResult{Stdout: []byte("contents")}, nil
	}}

	got, err := execfs.New(exec, "/workspace").ReadFile(
		context.Background(),
		"docs/a file.txt",
	)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "contents" {
		t.Fatalf("ReadFile() = %q, want %q", got, "contents")
	}
}

func TestFS_WriteFile(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{run: func(req sandbox.ExecRequest) (sandbox.ExecResult, error) {
		wantCmd := []string{
			"sh",
			"-c",
			writeScript,
			"execfs",
			"/workspace/nested",
			"/workspace/nested/file.txt",
		}
		if !reflect.DeepEqual(req.Cmd, wantCmd) {
			t.Fatalf("command = %#v, want %#v", req.Cmd, wantCmd)
		}
		if string(req.Stdin) != "new contents" {
			t.Fatalf("stdin = %q, want %q", req.Stdin, "new contents")
		}
		return sandbox.ExecResult{}, nil
	}}

	err := execfs.New(exec, "/workspace").WriteFile(
		context.Background(),
		"nested/file.txt",
		[]byte("new contents"),
	)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestFS_Stat(t *testing.T) {
	t.Parallel()

	exec := &fakeExec{run: func(req sandbox.ExecRequest) (sandbox.ExecResult, error) {
		wantCmd := []string{"sh", "-c", statScript, "execfs", "/workspace/dir"}
		if !reflect.DeepEqual(req.Cmd, wantCmd) {
			t.Fatalf("command = %#v, want %#v", req.Cmd, wantCmd)
		}
		return sandbox.ExecResult{Stdout: []byte("42\t1700000000\t41ed\n")}, nil
	}}

	info, err := execfs.New(exec, "/workspace").Stat(context.Background(), "dir")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Name() != "dir" {
		t.Errorf("Name() = %q, want %q", info.Name(), "dir")
	}
	if info.Size() != 42 {
		t.Errorf("Size() = %d, want 42", info.Size())
	}
	if info.Mode() != fs.ModeDir|0o755 {
		t.Errorf("Mode() = %v, want %v", info.Mode(), fs.ModeDir|0o755)
	}
	if !info.ModTime().Equal(time.Unix(1700000000, 0)) {
		t.Errorf("ModTime() = %v, want %v", info.ModTime(), time.Unix(1700000000, 0))
	}
}

func TestFS_RejectsPathsOutsideWorkspace(t *testing.T) {
	t.Parallel()

	paths := []string{
		"../etc/passwd",
		"dir/../file",
		"/etc/passwd",
	}
	for _, filePath := range paths {
		t.Run(filePath, func(t *testing.T) {
			t.Parallel()

			exec := &fakeExec{run: func(sandbox.ExecRequest) (sandbox.ExecResult, error) {
				t.Fatal("executor called for rejected path")
				return sandbox.ExecResult{}, nil
			}}

			_, err := execfs.New(exec, "/workspace").ReadFile(context.Background(), filePath)
			if err == nil {
				t.Fatal("ReadFile() error = nil, want path jail error")
			}
			if len(exec.calls) != 0 {
				t.Fatalf("executor calls = %d, want 0", len(exec.calls))
			}
		})
	}
}

func TestFS_ReportsExecutorFailures(t *testing.T) {
	t.Parallel()

	t.Run("run error", func(t *testing.T) {
		t.Parallel()

		runErr := errors.New("transport unavailable")
		exec := &fakeExec{run: func(sandbox.ExecRequest) (sandbox.ExecResult, error) {
			return sandbox.ExecResult{}, runErr
		}}

		_, err := execfs.New(exec, "/workspace").ReadFile(context.Background(), "file")
		if !errors.Is(err, runErr) {
			t.Fatalf("ReadFile() error = %v, want wrapping %v", err, runErr)
		}
	})

	t.Run("non-zero exit", func(t *testing.T) {
		t.Parallel()

		exec := &fakeExec{run: func(sandbox.ExecRequest) (sandbox.ExecResult, error) {
			return sandbox.ExecResult{ExitCode: 1, Stderr: []byte("not found\n")}, nil
		}}

		_, err := execfs.New(exec, "/workspace").ReadFile(context.Background(), "missing")
		if err == nil {
			t.Fatal("ReadFile() error = nil, want command failure")
		}
	})
}
