package execfs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/execfs"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type captureExec struct {
	last sandboxcore.ExecRequest
}

func (c *captureExec) Run(_ context.Context, req sandboxcore.ExecRequest) (sandboxcore.ExecResult, error) {
	c.last = req
	return sandboxcore.ExecResult{ExitCode: 0}, nil
}

func TestJailedPathAcceptsAbsoluteUnderRoot(t *testing.T) {
	exec := &captureExec{}
	fsys := execfs.New(exec, "/workspace", nil)
	if _, err := fsys.ReadFile(context.Background(), "/workspace/root.json"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/workspace/root.json" {
		t.Fatalf("path = %q", got)
	}
}

func TestJailedPathAcceptsRootItself(t *testing.T) {
	exec := &captureExec{}
	fsys := execfs.New(exec, "/workspace", nil)
	if _, err := fsys.ReadFile(context.Background(), "/workspace"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/workspace" {
		t.Fatalf("path = %q", got)
	}
}

func TestJailedPathAcceptsRelative(t *testing.T) {
	exec := &captureExec{}
	fsys := execfs.New(exec, "/workspace", nil)
	if _, err := fsys.ReadFile(context.Background(), "./root.json"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/workspace/root.json" {
		t.Fatalf("path = %q", got)
	}
}

func TestJailedPathRejectsOutsideAbsolute(t *testing.T) {
	fsys := execfs.New(&captureExec{}, "/workspace", nil)
	_, err := fsys.ReadFile(context.Background(), "/not-workdir")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("err = %v", err)
	}
}

func TestJailedPathRejectsDotDotEscape(t *testing.T) {
	fsys := execfs.New(&captureExec{}, "/workspace", nil)
	_, err := fsys.ReadFile(context.Background(), "../root.json")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("err = %v", err)
	}
}

type scriptedExec struct {
	last   sandboxcore.ExecRequest
	stdout []byte
}

func (s *scriptedExec) Run(_ context.Context, req sandboxcore.ExecRequest) (sandboxcore.ExecResult, error) {
	s.last = req
	return sandboxcore.ExecResult{ExitCode: 0, Stdout: s.stdout}, nil
}

func TestReadDirParsesStatListing(t *testing.T) {
	exec := &scriptedExec{stdout: []byte("12\t1700000000\t81a4\t/workspace/index.html\n0\t1700000001\t41ed\t/workspace/src\n")}
	fsys := execfs.New(exec, "/workspace", nil)
	entries, err := fsys.ReadDir(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Name != "index.html" || entries[0].IsDir || entries[0].Size != 12 {
		t.Fatalf("file = %+v", entries[0])
	}
	if entries[1].Name != "src" || !entries[1].IsDir {
		t.Fatalf("dir = %+v", entries[1])
	}
}

func TestReadDirMkdirRemoveUseJailedCommandContract(t *testing.T) {
	exec := &captureExec{}
	fsys := execfs.New(exec, "/workspace", nil)

	if _, err := fsys.ReadDir(context.Background(), "src"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/workspace/src" {
		t.Fatalf("ReadDir path = %q", got)
	}
	if !strings.Contains(exec.last.Cmd[2], "find") {
		t.Fatalf("ReadDir script = %q", exec.last.Cmd[2])
	}

	if err := fsys.Mkdir(context.Background(), "src/lib"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/workspace/src/lib" {
		t.Fatalf("Mkdir path = %q", got)
	}
	if !strings.Contains(exec.last.Cmd[2], "mkdir") {
		t.Fatalf("Mkdir script = %q", exec.last.Cmd[2])
	}

	if err := fsys.Remove(context.Background(), "src/lib"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/workspace/src/lib" {
		t.Fatalf("Remove path = %q", got)
	}
	if !strings.Contains(exec.last.Cmd[2], "rmdir") {
		t.Fatalf("Remove script = %q", exec.last.Cmd[2])
	}
}

func TestDirOpsRejectEscapes(t *testing.T) {
	fsys := execfs.New(&captureExec{}, "/workspace", nil)
	if _, err := fsys.ReadDir(context.Background(), "../etc"); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("ReadDir err = %v", err)
	}
	if err := fsys.Mkdir(context.Background(), "../etc"); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("Mkdir err = %v", err)
	}
	if err := fsys.Remove(context.Background(), "../etc/passwd"); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("Remove err = %v", err)
	}
}

func TestJailedPathWhitelistAllowsExtraTargetAndDeniesWrites(t *testing.T) {
	exec := &captureExec{}
	fsys := execfs.New(exec, "/workspace", &sandboxcore.PathPolicy{Grants: []sandboxcore.PathGrant{
		{Path: "/workspace", Read: true, Write: false},
		{Path: "/cache", Read: true, Write: true},
	}})
	if _, err := fsys.ReadFile(context.Background(), "/cache/blob"); err != nil {
		t.Fatal(err)
	}
	if got := exec.last.Cmd[len(exec.last.Cmd)-1]; got != "/cache/blob" {
		t.Fatalf("path = %q", got)
	}
	if err := fsys.WriteFile(context.Background(), "notes.txt", []byte("x")); err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("readonly err = %v", err)
	}
	if _, err := fsys.ReadFile(context.Background(), "/etc/passwd"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("outside err = %v", err)
	}
}
