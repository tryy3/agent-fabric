package planeconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/planeconfig"
)

func TestLoadFileEngine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"dataDir":"./data","docker":{"runtime":"auto"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, deprecated, err := planeconfig.LoadFile(path)
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
	_, _, err := planeconfig.LoadFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "read plane config") {
		t.Fatalf("err = %v", err)
	}
}
