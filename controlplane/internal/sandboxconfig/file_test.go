package sandboxconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func TestLoadFileEngine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sandbox.json")
	if err := os.WriteFile(path, []byte(`{"dataDir":"./data","docker":{"runtime":"auto"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, deprecated, err := sandboxconfig.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if engine.DataDir != "./data" || engine.Docker.Runtime != "auto" {
		t.Fatalf("engine = %+v", engine)
	}
	if deprecated.HasKeys() {
		t.Fatalf("deprecated = %+v", deprecated)
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, _, err := sandboxconfig.LoadFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "read sandbox config") {
		t.Fatalf("err = %v", err)
	}
}
