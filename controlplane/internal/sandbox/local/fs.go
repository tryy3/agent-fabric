package local

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type localFS struct {
	root   string
	policy *sandboxcore.PathPolicy
}

func (f *localFS) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := sandboxcore.ResolveOS(f.root, path, f.policy, sandboxcore.PathRead)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (f *localFS) WriteFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := sandboxcore.ResolveOS(f.root, path, f.policy, sandboxcore.PathWrite)
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
	resolved, err := sandboxcore.ResolveOS(f.root, path, f.policy, sandboxcore.PathRead)
	if err != nil {
		return nil, err
	}
	return os.Stat(resolved)
}

func (f *localFS) ReadDir(ctx context.Context, path string) ([]sandboxcore.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := sandboxcore.ResolveOS(f.root, path, f.policy, sandboxcore.PathRead)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	out := make([]sandboxcore.DirEntry, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		out = append(out, sandboxcore.DirEntry{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return out, nil
}

func (f *localFS) Mkdir(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := sandboxcore.ResolveOS(f.root, path, f.policy, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	return os.MkdirAll(resolved, 0o755)
}

func (f *localFS) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := sandboxcore.ResolveOS(f.root, path, f.policy, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	return os.Remove(resolved)
}
