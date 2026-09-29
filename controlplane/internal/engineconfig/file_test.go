package engineconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/engineconfig"
)

func TestLoadFileEngine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"dataDir":"./data","docker":{"runtime":"auto"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, deprecated, err := engineconfig.LoadFile(path)
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
	_, _, err := engineconfig.LoadFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "read control plane config") {
		t.Fatalf("err = %v", err)
	}
}
