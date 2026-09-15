package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveUnderRoot(root, userPath string) (string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		rootResolved = cleanRoot
	}

	var candidate string
	if filepath.IsAbs(userPath) {
		candidate = filepath.Clean(userPath)
	} else {
		candidate = filepath.Join(cleanRoot, userPath)
		candidate = filepath.Clean(candidate)
	}

	resolved, err := evalExistingPrefix(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace root %q", userPath, cleanRoot)
	}
	return candidate, nil
}

// evalExistingPrefix EvalSymlinks the longest existing prefix, then rejoins the tail.
func evalExistingPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	cur := path
	var tail []string
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		dir, base := filepath.Dir(cur), filepath.Base(cur)
		if dir == cur {
			return path, nil
		}
		tail = append(tail, base)
		cur = dir
	}
}
