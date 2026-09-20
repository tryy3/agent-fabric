package sandboxconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// Load maps sandbox.json into host engine defaults: kind, runtime, binPath,
// default image, and fallback containerScope for prompts that are not bound to
// a project. Per-prompt Open applies project isolation on top of these defaults.

type config struct {
	Kind          string        `json:"kind"`
	WorkspaceRoot string        `json:"workspaceRoot"`
	Docker        *dockerConfig `json:"docker"`
}

type dockerConfig struct {
	ContainerScope string        `json:"containerScope"`
	IdleTTLSeconds int64         `json:"idleTTLSeconds"`
	Runtime        string        `json:"runtime"`
	BinPath        string        `json:"binPath"`
	Image          string        `json:"image"`
	Dockerfile     string        `json:"dockerfile"`
	BuildContext   string        `json:"buildContext"`
	Mounts         []mountConfig `json:"mounts"`
}

type mountConfig struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

func Load(configDir string, data []byte) (sandbox.OpenOptions, error) {
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return sandbox.OpenOptions{}, fmt.Errorf("decode sandbox config: %w", err)
	}
	if cfg.Kind == "" {
		return sandbox.OpenOptions{}, fmt.Errorf("sandbox kind is required")
	}
	if strings.TrimSpace(cfg.WorkspaceRoot) == "" {
		return sandbox.OpenOptions{}, fmt.Errorf("sandbox workspace root is required")
	}

	opts := sandbox.OpenOptions{
		Kind:          cfg.Kind,
		WorkspaceRoot: cfg.WorkspaceRoot,
	}
	if cfg.Kind != "docker" {
		return opts, nil
	}
	if cfg.Docker == nil {
		return sandbox.OpenOptions{}, fmt.Errorf("docker options are required")
	}
	if (cfg.Docker.Image == "") == (cfg.Docker.Dockerfile == "") {
		return sandbox.OpenOptions{}, fmt.Errorf("docker requires exactly one of image or dockerfile")
	}

	scope := sandbox.ScopeSession
	switch cfg.Docker.ContainerScope {
	case "", string(sandbox.ScopeSession):
	case string(sandbox.ScopeShared):
		scope = sandbox.ScopeShared
	default:
		return sandbox.OpenOptions{}, fmt.Errorf(
			"invalid docker container scope %q",
			cfg.Docker.ContainerScope,
		)
	}

	dockerfile := resolveRelative(configDir, cfg.Docker.Dockerfile)
	mounts := make([]sandbox.Mount, len(cfg.Docker.Mounts))
	for i, mount := range cfg.Docker.Mounts {
		mounts[i] = sandbox.Mount{
			Source:   resolveRelative(configDir, mount.Source),
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		}
	}

	opts.Docker = &sandbox.DockerOptions{
		Scope:        sandbox.Scope{Kind: scope},
		IdleTTL:      time.Duration(cfg.Docker.IdleTTLSeconds) * time.Second,
		Runtime:      cfg.Docker.Runtime,
		BinPath:      cfg.Docker.BinPath,
		Image:        cfg.Docker.Image,
		Dockerfile:   dockerfile,
		BuildContext: cfg.Docker.BuildContext,
		Mounts:       mounts,
	}
	return opts, nil
}

func resolveRelative(configDir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(configDir, path)
}
