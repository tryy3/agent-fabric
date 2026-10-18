package skills

import (
	"context"
	"io/fs"
	"path"
)

// Reader is the read-only view of a skills source: an embedded folder, a
// directory on disk, or a project workspace.
type Reader interface {
	ReadFile(ctx context.Context, name string) ([]byte, error)
	ReadDir(ctx context.Context, name string) ([]DirEntry, error)
}

// DirEntry is one directory listing entry.
type DirEntry struct {
	Name  string
	IsDir bool
}

// FSReader adapts an io/fs.FS (embed.FS, os.DirFS, fstest.MapFS) to Reader.
type FSReader struct{ FS fs.FS }

func (r FSReader) ReadFile(_ context.Context, name string) ([]byte, error) {
	return fs.ReadFile(r.FS, path.Clean(name))
}

func (r FSReader) ReadDir(_ context.Context, name string) ([]DirEntry, error) {
	entries, err := fs.ReadDir(r.FS, path.Clean(name))
	if err != nil {
		return nil, err
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{Name: e.Name(), IsDir: e.IsDir()})
	}
	return out, nil
}
