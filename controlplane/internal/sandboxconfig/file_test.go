package sandboxconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func TestLoadFileLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sandbox.json")
	if err := os.WriteFile(path, []byte(`{"kind":"local","workspaceRoot":"/tmp/ws"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	opts, err := sandboxconfig.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Kind != "local" || opts.WorkspaceRoot != "/tmp/ws" {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestLoadFileResolvesRelativeMountAgainstFileDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sandbox.json")
	body := `{"kind":"docker","workspaceRoot":"/workspace","docker":{"image":"alpine","mounts":[{"source":"./data","target":"/workspace"}]}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	opts, err := sandboxconfig.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker.Mounts[0].Source != filepath.Join(dir, "data") {
		t.Fatalf("mount source = %q", opts.Docker.Mounts[0].Source)
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeSession {
		t.Fatalf("scope = %q", opts.Docker.Scope.Kind)
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, err := sandboxconfig.LoadFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "read sandbox config") {
		t.Fatalf("err = %v", err)
	}
}
