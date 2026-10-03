package local

import (
	"context"
	"errors"
	"fmt"
	"io"
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

func (f *localFS) Rename(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, dst, err := f.resolvePair(from, sandboxcore.PathWrite, to, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	if err := requireFreeDestination(dst); err != nil {
		return err
	}
	return renameNoReplace(src, dst)
}

// renameNoReplace moves src to dst without ever replacing an existing file.
// os.Rename silently overwrites, so regular files go through link+unlink,
// where link fails atomically when dst appeared after the earlier check.
func renameNoReplace(src, dst string) error {
	if info, err := os.Lstat(src); err == nil && info.Mode().IsRegular() {
		lerr := os.Link(src, dst)
		if lerr == nil {
			return os.Remove(src)
		}
		if errors.Is(lerr, fs.ErrExist) {
			return fmt.Errorf("%s: %w", filepath.Base(dst), fs.ErrExist)
		}
	}
	return os.Rename(src, dst)
}

func (f *localFS) Copy(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, dst, err := f.resolvePair(from, sandboxcore.PathRead, to, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	if err := requireFreeDestination(dst); err != nil {
		return err
	}
	return copyTree(ctx, src, dst)
}

func (f *localFS) resolvePair(
	from string, fromAccess sandboxcore.PathAccess,
	to string, toAccess sandboxcore.PathAccess,
) (string, string, error) {
	src, err := sandboxcore.ResolveOS(f.root, from, f.policy, fromAccess)
	if err != nil {
		return "", "", err
	}
	dst, err := sandboxcore.ResolveOS(f.root, to, f.policy, toAccess)
	if err != nil {
		return "", "", err
	}
	return src, dst, nil
}

func requireFreeDestination(dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%s: %w", filepath.Base(dst), fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported: %s", filepath.Base(p))
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Mkdir(target, info.Mode().Perm()|0o700)
		}
		return copyFile(p, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
