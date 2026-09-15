package sandboxconfig_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func TestLoadDockerDockerfile(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
	  "kind": "docker",
	  "workspaceRoot": "/workspace",
	  "docker": {
	    "containerScope": "session",
	    "idleTTLSeconds": 600,
	    "runtime": "auto",
	    "binPath": "/usr/bin/docker",
	    "dockerfile": "./Dockerfile",
	    "buildContext": "./context",
	    "mounts": [
	      {"source": "./data", "target": "/workspace", "readOnly": true},
	      {"source": "/host/cache", "target": "/cache"}
	    ]
	  }
	}`)

	opts, err := sandboxconfig.Load(dir, data)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Kind != "docker" || opts.WorkspaceRoot != "/workspace" {
		t.Fatalf("options = %+v", opts)
	}
	if opts.Docker == nil {
		t.Fatal("docker options are nil")
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeSession || opts.Docker.Scope.SessionID != "" {
		t.Fatalf("scope = %+v", opts.Docker.Scope)
	}
	if opts.Docker.IdleTTL != 600*time.Second {
		t.Fatalf("ttl = %v", opts.Docker.IdleTTL)
	}
	if opts.Docker.Runtime != "auto" || opts.Docker.BinPath != "/usr/bin/docker" {
		t.Fatalf("engine = %+v", opts.Docker)
	}
	if opts.Docker.Dockerfile != filepath.Join(dir, "Dockerfile") {
		t.Fatalf("dockerfile = %q", opts.Docker.Dockerfile)
	}
	if opts.Docker.BuildContext != "./context" {
		t.Fatalf("build context = %q", opts.Docker.BuildContext)
	}
	if got := opts.Docker.Mounts[0]; got.Source != filepath.Join(dir, "data") ||
		got.Target != "/workspace" || !got.ReadOnly {
		t.Fatalf("relative mount = %+v", got)
	}
	if got := opts.Docker.Mounts[1]; got.Source != "/host/cache" {
		t.Fatalf("absolute mount = %+v", got)
	}
}

func TestLoadDockerDefaultsSessionScopeAndZeroTTL(t *testing.T) {
	opts, err := sandboxconfig.Load(t.TempDir(), []byte(`{
	  "kind":"docker",
	  "workspaceRoot":"/workspace",
	  "docker":{"image":"alpine"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeSession {
		t.Fatalf("scope = %q", opts.Docker.Scope.Kind)
	}
	if opts.Docker.IdleTTL != 0 {
		t.Fatalf("ttl = %v", opts.Docker.IdleTTL)
	}
}

func TestLoadDockerSharedImage(t *testing.T) {
	opts, err := sandboxconfig.Load(t.TempDir(), []byte(`{
	  "kind":"docker",
	  "workspaceRoot":"/workspace",
	  "docker":{"containerScope":"shared","image":"alpine"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker.Scope.Kind != sandbox.ScopeShared || opts.Docker.Image != "alpine" {
		t.Fatalf("docker options = %+v", opts.Docker)
	}
}

func TestLoadRejectsInvalidDockerConfig(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"missing docker options", `{"kind":"docker","workspaceRoot":"/w"}`},
		{"image and dockerfile", `{
		  "kind":"docker","workspaceRoot":"/w",
		  "docker":{"containerScope":"shared","image":"alpine","dockerfile":"./Dockerfile"}
		}`},
		{"missing image and dockerfile", `{
		  "kind":"docker","workspaceRoot":"/w","docker":{"containerScope":"shared"}
		}`},
		{"invalid scope", `{
		  "kind":"docker","workspaceRoot":"/w",
		  "docker":{"containerScope":"process","image":"alpine"}
		}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := sandboxconfig.Load(t.TempDir(), []byte(test.data)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadLocal(t *testing.T) {
	opts, err := sandboxconfig.Load(t.TempDir(), []byte(`{
	  "kind":"local",
	  "workspaceRoot":"/tmp/ws"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if opts.Kind != "local" || opts.WorkspaceRoot != "/tmp/ws" {
		t.Fatalf("options = %+v", opts)
	}
	if opts.Docker != nil {
		t.Fatal("docker should be nil")
	}
}

func TestLoadRejectsMissingKind(t *testing.T) {
	if _, err := sandboxconfig.Load(t.TempDir(), []byte(`{"workspaceRoot":"/tmp/ws"}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsMalformedJSON(t *testing.T) {
	if _, err := sandboxconfig.Load(t.TempDir(), []byte(`{"kind":`)); err == nil {
		t.Fatal("expected error")
	}
}
