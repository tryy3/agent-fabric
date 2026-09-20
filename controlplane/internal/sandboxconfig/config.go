package sandboxconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// Engine is the process-host config from sandbox.json: database, listen
// address, local dataDir, and which docker/podman binary to exec.
type Engine struct {
	DatabaseURL string
	ListenAddr  string
	DataDir     string
	Docker      DockerEngine
}

type DockerEngine struct {
	Runtime        string
	BinPath        string
	IdentityPrefix string
}

type fileConfig struct {
	DatabaseURL   string        `json:"databaseUrl"`
	ListenAddr    string        `json:"listenAddr"`
	DataDir       string        `json:"dataDir"`
	Kind          string        `json:"kind"`
	WorkspaceRoot string        `json:"workspaceRoot"`
	Docker        *dockerConfig `json:"docker"`
}

type dockerConfig struct {
	Runtime        string `json:"runtime"`
	BinPath        string `json:"binPath"`
	IdentityPrefix string `json:"identityPrefix"`
	Image          string `json:"image"`
	Dockerfile     string `json:"dockerfile"`
	BuildContext   string `json:"buildContext"`
	IdleTTLSeconds int64  `json:"idleTTLSeconds"`
	ContainerScope string `json:"containerScope"`
}

// Load reads engine keys from sandbox.json. Deprecated overlay keys
// (kind, workspaceRoot, image, idleTTLSeconds, dockerfile) are returned for
// one-time migration into plane_settings and are not used as OpenOptions.
func Load(data []byte) (Engine, catalog.DeprecatedSandbox, error) {
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Engine{}, catalog.DeprecatedSandbox{}, fmt.Errorf("decode sandbox config: %w", err)
	}

	engine := Engine{
		DatabaseURL: strings.TrimSpace(cfg.DatabaseURL),
		ListenAddr:  strings.TrimSpace(cfg.ListenAddr),
		DataDir:     strings.TrimSpace(cfg.DataDir),
	}
	if cfg.Docker != nil {
		engine.Docker = DockerEngine{
			Runtime:        cfg.Docker.Runtime,
			BinPath:        cfg.Docker.BinPath,
			IdentityPrefix: cfg.Docker.IdentityPrefix,
		}
	}
	if engine.DataDir == "" && cfg.Kind == "local" && strings.TrimSpace(cfg.WorkspaceRoot) != "" {
		engine.DataDir = cfg.WorkspaceRoot
	}
	if engine.DataDir == "" {
		engine.DataDir = "./data"
	}

	deprecated := catalog.DeprecatedSandbox{
		Kind:           cfg.Kind,
		WorkspaceRoot:  strings.TrimSpace(cfg.WorkspaceRoot),
		Image:          "",
		IdleTTLSeconds: 0,
	}
	if cfg.Docker != nil {
		deprecated.Image = cfg.Docker.Image
		deprecated.Dockerfile = cfg.Docker.Dockerfile
		deprecated.BuildContext = cfg.Docker.BuildContext
		deprecated.IdleTTLSeconds = cfg.Docker.IdleTTLSeconds
	}
	return engine, deprecated, nil
}
