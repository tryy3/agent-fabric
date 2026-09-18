package execfs

// Command contract (kept stable for executor fakes and container images):
//
//	ReadFile:  ["sh", "-c", `cat -- "$1"`, "execfs", absolutePath]
//	WriteFile: ["sh", "-c", writeScript, "execfs", parentDir, absolutePath]
//	Stat:      ["sh", "-c", `stat -c '%s\t%Y\t%f' -- "$1"`, "execfs", absolutePath]
//
// writeScript is exactly:
//
//	set -eu
//	dir=$1
//	dst=$2
//	mkdir -p -- "$dir"
//	tmp=$(mktemp "$dir/.execfs.XXXXXX")
//	trap 'rm -f -- "$tmp"' EXIT HUP INT TERM
//	cat > "$tmp"
//	mv -f -- "$tmp" "$dst"
//	trap - EXIT HUP INT TERM

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

const (
	readScript  = `cat -- "$1"`
	writeScript = `set -eu
dir=$1
dst=$2
mkdir -p -- "$dir"
tmp=$(mktemp "$dir/.execfs.XXXXXX")
trap 'rm -f -- "$tmp"' EXIT HUP INT TERM
cat > "$tmp"
mv -f -- "$tmp" "$dst"
trap - EXIT HUP INT TERM`
	statScript = `stat -c '%s	%Y	%f' -- "$1"`
)

type execFS struct {
	exec          sandboxcore.Executor
	workspaceRoot string
}

// New creates a filesystem that performs operations through exec.
func New(exec sandboxcore.Executor, workspaceRoot string) sandboxcore.FS {
	return &execFS{
		exec:          exec,
		workspaceRoot: path.Clean(workspaceRoot),
	}
}

func (f *execFS) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	fullPath, err := f.jailedPath(filePath)
	if err != nil {
		return nil, err
	}

	result, err := f.run(ctx, sandboxcore.ExecRequest{
		Cmd: []string{"sh", "-c", readScript, "execfs", fullPath},
	})
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return result.Stdout, nil
}

func (f *execFS) WriteFile(ctx context.Context, filePath string, data []byte) error {
	fullPath, err := f.jailedPath(filePath)
	if err != nil {
		return err
	}

	_, err = f.run(ctx, sandboxcore.ExecRequest{
		Cmd: []string{
			"sh",
			"-c",
			writeScript,
			"execfs",
			path.Dir(fullPath),
			fullPath,
		},
		Stdin: data,
	})
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (f *execFS) Stat(ctx context.Context, filePath string) (fs.FileInfo, error) {
	fullPath, err := f.jailedPath(filePath)
	if err != nil {
		return nil, err
	}

	result, err := f.run(ctx, sandboxcore.ExecRequest{
		Cmd: []string{"sh", "-c", statScript, "execfs", fullPath},
	})
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	info, err := parseFileInfo(filePath, result.Stdout)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	return info, nil
}

func (f *execFS) jailedPath(filePath string) (string, error) {
	return sandboxcore.ContainUnderRootPOSIX(f.workspaceRoot, filePath)
}

func (f *execFS) run(
	ctx context.Context,
	req sandboxcore.ExecRequest,
) (sandboxcore.ExecResult, error) {
	result, err := f.exec.Run(ctx, req)
	if err != nil {
		return sandboxcore.ExecResult{}, fmt.Errorf("run command: %w", err)
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(string(result.Stderr))
		if detail == "" {
			detail = "no stderr"
		}
		return sandboxcore.ExecResult{}, fmt.Errorf(
			"command exited with code %d: %s",
			result.ExitCode,
			detail,
		)
	}
	return result, nil
}

type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func parseFileInfo(filePath string, output []byte) (fs.FileInfo, error) {
	fields := strings.Fields(string(output))
	if len(fields) != 3 {
		return nil, fmt.Errorf("invalid stat output %q", output)
	}

	size, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse size: %w", err)
	}
	modUnix, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse modification time: %w", err)
	}
	unixMode, err := strconv.ParseUint(fields[2], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("parse mode: %w", err)
	}

	return fileInfo{
		name:    path.Base(path.Clean(filePath)),
		size:    size,
		mode:    fileMode(unixMode),
		modTime: time.Unix(modUnix, 0),
	}, nil
}

func fileMode(unixMode uint64) fs.FileMode {
	mode := fs.FileMode(unixMode & 0o777)
	switch unixMode & 0o170000 {
	case 0o040000:
		mode |= fs.ModeDir
	case 0o120000:
		mode |= fs.ModeSymlink
	case 0o010000:
		mode |= fs.ModeNamedPipe
	case 0o140000:
		mode |= fs.ModeSocket
	case 0o060000:
		mode |= fs.ModeDevice
	case 0o020000:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	}
	return mode
}

func (i fileInfo) Name() string       { return i.name }
func (i fileInfo) Size() int64        { return i.size }
func (i fileInfo) Mode() fs.FileMode  { return i.mode }
func (i fileInfo) ModTime() time.Time { return i.modTime }
func (i fileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fileInfo) Sys() any           { return nil }
