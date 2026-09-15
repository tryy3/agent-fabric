package local

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type localFS struct {
	root string
}

func (f *localFS) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := sandboxcore.ResolveUnderRoot(f.root, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (f *localFS) WriteFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := sandboxcore.ResolveUnderRoot(f.root, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	return os.WriteFile(resolved, data, 0o644)
}

func (f *localFS) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := sandboxcore.ResolveUnderRoot(f.root, path)
	if err != nil {
		return nil, err
	}
	return os.Stat(resolved)
}
