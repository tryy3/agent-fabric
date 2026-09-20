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
	return sandboxcore.ExecResult{ExitCode: 0, Stdout: []byte("ok")}, nil
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
