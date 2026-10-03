// Package fsops holds the project path operations (move, copy, duplicate) that
// sit above the raw sandbox.FS primitives.
//
// It is the single place that decides what a valid move or copy is: path
// normalization, root/self/descendant checks, destination-parent checks,
// collision-free duplicate naming, and error classification. Callers are thin
// adapters over it: the catalog HTTP handlers today, and the agent file tools
// (#67) or a command palette later. Callers that need extra protection (agent
// path gates, protected paths such as .git) supply a Guard instead of forking
// the logic; the Guard runs before any mutation.
package fsops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// Op names the mutation a Guard is asked to approve.
type Op string

const (
	OpMove Op = "move"
	OpCopy Op = "copy"
)

var (
	// ErrNotFound and ErrExists alias the io/fs sentinels so both fsops and
	// raw FS errors classify identically with errors.Is.
	ErrNotFound = fs.ErrNotExist
	ErrExists   = fs.ErrExist

	// ErrInvalidPath covers empty/root paths, identical source and
	// destination, and a destination whose parent is not a directory.
	ErrInvalidPath = errors.New("invalid path")
	// ErrEscapesRoot is returned for paths that climb out of the project root.
	ErrEscapesRoot = errors.New("path escapes project root")
	// ErrIntoSelf is returned when a directory would be moved or copied into
	// itself or one of its descendants.
	ErrIntoSelf = errors.New("cannot move or copy a directory into itself")
	// ErrProtected is the conventional error for a Guard to return when a path
	// is off limits.
	ErrProtected = errors.New("path is protected")
)

// Guard approves or vetoes an operation before it runs. from and to are the
// normalized project-relative paths. A non-nil error aborts the operation and
// is returned unchanged.
type Guard func(ctx context.Context, op Op, from, to string) error

// Service performs path operations against one project's filesystem.
type Service struct {
	fs    sandbox.FS
	guard Guard
}

// Option configures a Service.
type Option func(*Service)

// WithGuard installs a Guard.
func WithGuard(g Guard) Option { return func(s *Service) { s.guard = g } }

// New returns a Service over fsys.
func New(fsys sandbox.FS, opts ...Option) *Service {
	s := &Service{fs: fsys}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Move renames or relocates from to the new path to and returns the
// normalized destination. It never overwrites.
func (s *Service) Move(ctx context.Context, from, to string) (string, error) {
	return s.transfer(ctx, OpMove, from, to)
}

// Copy duplicates from to the explicit path to and returns the normalized
// destination. It never overwrites.
func (s *Service) Copy(ctx context.Context, from, to string) (string, error) {
	return s.transfer(ctx, OpCopy, from, to)
}

// Duplicate copies from next to itself under a free "name copy.ext" name
// ("name copy 2.ext", ... when taken) and returns the new path.
func (s *Service) Duplicate(ctx context.Context, from string) (string, error) {
	src, err := Normalize(from)
	if err != nil {
		return "", err
	}
	parent := path.Dir(src)
	entries, err := s.fs.ReadDir(ctx, parent)
	if err != nil {
		return "", classify(err, parent)
	}
	taken := make(map[string]bool, len(entries))
	for _, e := range entries {
		taken[e.Name] = true
	}
	for attempt := 1; attempt <= maxDuplicateAttempts; attempt++ {
		name := DuplicateName(path.Base(src), attempt)
		if taken[name] {
			continue
		}
		dst := path.Join(parent, name)
		if _, err := s.transfer(ctx, OpCopy, src, dst); err != nil {
			if errors.Is(err, ErrExists) {
				// Lost a race for this name; try the next one.
				continue
			}
			return "", err
		}
		return dst, nil
	}
	return "", fmt.Errorf("%w: no free duplicate name for %q", ErrExists, src)
}

const maxDuplicateAttempts = 1000

// DuplicateName returns the nth candidate duplicate name for base: n=1 is
// "name copy.ext", n=2 is "name copy 2.ext". A leading dot is not treated as
// an extension separator (".env" -> ".env copy").
func DuplicateName(base string, n int) string {
	ext := path.Ext(base)
	if ext == base {
		ext = ""
	}
	stem := strings.TrimSuffix(base, ext)
	suffix := " copy"
	if n > 1 {
		suffix += " " + strconv.Itoa(n)
	}
	return stem + suffix + ext
}

// Normalize converts a user path into a clean project-relative path without a
// leading slash. The project root and anything escaping it are rejected.
func Normalize(p string) (string, error) {
	cleaned := path.Clean(strings.TrimLeft(strings.TrimSpace(p), "/"))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: %q", ErrEscapesRoot, p)
	}
	if cleaned == "." {
		return "", fmt.Errorf("%w: the project root cannot be moved or copied", ErrInvalidPath)
	}
	return cleaned, nil
}

func (s *Service) transfer(ctx context.Context, op Op, from, to string) (string, error) {
	src, err := Normalize(from)
	if err != nil {
		return "", err
	}
	dst, err := Normalize(to)
	if err != nil {
		return "", err
	}
	if src == dst {
		return "", fmt.Errorf("%w: source and destination are the same", ErrInvalidPath)
	}
	if strings.HasPrefix(dst, src+"/") {
		return "", fmt.Errorf("%w: %q into %q", ErrIntoSelf, src, dst)
	}
	if s.guard != nil {
		if err := s.guard(ctx, op, src, dst); err != nil {
			return "", err
		}
	}
	if _, err := s.fs.Stat(ctx, src); err != nil {
		return "", classify(err, src)
	}
	parent := path.Dir(dst)
	if parent != "." {
		info, err := s.fs.Stat(ctx, parent)
		if err != nil {
			return "", classify(err, parent)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%w: %q is not a directory", ErrInvalidPath, parent)
		}
	}
	if op == OpMove {
		err = s.fs.Rename(ctx, src, dst)
	} else {
		err = s.fs.Copy(ctx, src, dst)
	}
	if err != nil {
		return "", classify(err, dst)
	}
	return dst, nil
}

// classify adds the offending path to not-found/exists errors so messages are
// useful, while keeping errors.Is working. Other errors pass through.
func classify(err error, p string) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return fmt.Errorf("%w: %s", ErrNotFound, p)
	case errors.Is(err, ErrExists):
		return fmt.Errorf("%w: %s", ErrExists, p)
	}
	return err
}

// ProtectGit is a Guard that refuses to move or copy the repository metadata
// directory, or anything into or out of it.
func ProtectGit(_ context.Context, _ Op, from, to string) error {
	for _, p := range []string{from, to} {
		if p == ".git" || strings.HasPrefix(p, ".git/") {
			return fmt.Errorf("%w: %s", ErrProtected, p)
		}
	}
	return nil
}
