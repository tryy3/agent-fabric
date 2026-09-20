package sandboxcore

import (
	"context"
	"io/fs"
	"time"
)

type Capabilities struct {
	FS   bool
	Exec bool
}

func (c Capabilities) Satisfies(need Capabilities) bool {
	return (!need.FS || c.FS) && (!need.Exec || c.Exec)
}

type DirEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

type FS interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	Stat(ctx context.Context, path string) (fs.FileInfo, error)
	ReadDir(ctx context.Context, path string) ([]DirEntry, error)
	Mkdir(ctx context.Context, path string) error
	Remove(ctx context.Context, path string) error
}

type ExecRequest struct {
	Cmd     []string
	WorkDir string
	Stdin   []byte
	Timeout time.Duration
}

type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

type Executor interface {
	Run(ctx context.Context, req ExecRequest) (ExecResult, error)
}

type Environment interface {
	ID() string
	Caps() Capabilities
	FS() (FS, bool)
	Exec() (Executor, bool)
	Close(ctx context.Context) error
}

type ScopeKind string

const (
	ScopeShared  ScopeKind = "shared"
	ScopeSession ScopeKind = "session"
	ScopeProject ScopeKind = "project"

	DefaultSessionIdleTTL = 10 * time.Minute
	DefaultProjectIdleTTL = time.Hour

	MountBind   = "bind"
	MountVolume = "volume"
)

type Scope struct {
	Kind          ScopeKind
	SessionID     string
	ProjectID     string
	EnvironmentID string
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
	Type     string
}

type DockerOptions struct {
	Scope           Scope
	IdleTTL         time.Duration
	Runtime         string
	BinPath         string
	Image           string
	Dockerfile      string
	BuildContext    string
	Mounts          []Mount
	WorkspaceVolume string
	Name            string
}

type OpenOptions struct {
	Kind          string
	WorkspaceRoot string
	Docker        *DockerOptions
	PathPolicy    *PathPolicy
}

type PathAccess string

const (
	PathRead  PathAccess = "read"
	PathWrite PathAccess = "write"
	PathExec  PathAccess = "exec"
)

type PathGrant struct {
	Path  string
	Read  bool
	Write bool
	Exec  bool
}

type PathPolicy struct {
	Grants []PathGrant
}
