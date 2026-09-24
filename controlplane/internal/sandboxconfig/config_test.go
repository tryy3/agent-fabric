package sandboxconfig_test

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func TestLoadEngineKeys(t *testing.T) {
	engine, deprecated, err := sandboxconfig.Load([]byte(`{
	  "databaseUrl": "postgres://agent@localhost/db",
	  "listenAddr": ":9090",
	  "dataDir": "/var/lib/agent-fabric",
	  "docker": {
	    "runtime": "podman",
	    "binPath": "/usr/bin/podman",
	    "identityPrefix": "dev-"
	  }
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if engine.DatabaseURL != "postgres://agent@localhost/db" || engine.ListenAddr != ":9090" {
		t.Fatalf("engine = %+v", engine)
	}
	if engine.DataDir != "/var/lib/agent-fabric" {
		t.Fatalf("dataDir = %q", engine.DataDir)
	}
	if engine.Docker.Runtime != "podman" || engine.Docker.BinPath != "/usr/bin/podman" || engine.Docker.IdentityPrefix != "dev-" {
		t.Fatalf("docker engine = %+v", engine.Docker)
	}
	if deprecated.HasKeys() {
		t.Fatalf("deprecated = %+v", deprecated)
	}
}

func TestLoadExtractsDeprecatedOverlayKeys(t *testing.T) {
	engine, deprecated, err := sandboxconfig.Load([]byte(`{
	  "kind": "docker",
	  "workspaceRoot": "/workspace",
	  "docker": {
	    "runtime": "auto",
	    "image": "alpine:3.20",
	    "idleTTLSeconds": 600,
	    "containerScope": "session"
	  }
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if engine.DataDir != "./data" {
		t.Fatalf("dataDir = %q", engine.DataDir)
	}
	if engine.Docker.Runtime != "auto" {
		t.Fatalf("runtime = %q", engine.Docker.Runtime)
	}
	if !deprecated.HasKeys() || deprecated.Kind != "docker" || deprecated.Image != "alpine:3.20" {
		t.Fatalf("deprecated = %+v", deprecated)
	}
	if deprecated.IdleTTLSeconds != 600 || deprecated.WorkspaceRoot != "/workspace" {
		t.Fatalf("deprecated overlay = %+v", deprecated)
	}
}

func TestLoadLocalDeprecatedUsesWorkspaceRootAsDataDir(t *testing.T) {
	engine, deprecated, err := sandboxconfig.Load([]byte(`{
	  "kind":"local",
	  "workspaceRoot":"/tmp/ws"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if engine.DataDir != "/tmp/ws" {
		t.Fatalf("dataDir = %q", engine.DataDir)
	}
	if deprecated.Kind != "local" || deprecated.WorkspaceRoot != "/tmp/ws" {
		t.Fatalf("deprecated = %+v", deprecated)
	}
}

func TestLoadRejectsMalformedJSON(t *testing.T) {
	if _, _, err := sandboxconfig.Load([]byte(`{"kind":`)); err == nil {
		t.Fatal("expected error")
	}
}
