package sandboxcore

import (
	"fmt"
	"path"
	"strings"
)

// ContainUnderRootPOSIX resolves userPath under an absolute slash-path root.
// Absolute paths are allowed when they stay inside root; relative paths are
// joined to root. Escapes via ".." or absolute paths outside root error.
func ContainUnderRootPOSIX(root, userPath string) (string, error) {
	if !path.IsAbs(root) {
		return "", fmt.Errorf("workspace root must be absolute: %q", root)
	}
	cleanRoot := path.Clean(root)
	if userPath == "" {
		return cleanRoot, nil
	}
	candidate := userPath
	if !path.IsAbs(candidate) {
		candidate = path.Join(cleanRoot, candidate)
	}
	candidate = path.Clean(candidate)
	if !posixContained(cleanRoot, candidate) {
		return "", fmt.Errorf("path %q escapes workspace root %q", userPath, cleanRoot)
	}
	return candidate, nil
}

func posixContained(root, candidate string) bool {
	if candidate == root {
		return true
	}
	if root == "/" {
		return path.IsAbs(candidate)
	}
	return strings.HasPrefix(candidate, root+"/")
}
