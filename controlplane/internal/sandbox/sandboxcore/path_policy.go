package sandboxcore

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func ResolvePOSIX(root, userPath string, policy *PathPolicy, access PathAccess) (string, error) {
	if policy == nil {
		return ContainUnderRootPOSIX(root, userPath)
	}
	if !path.IsAbs(root) {
		return "", fmt.Errorf("workspace root must be absolute: %q", root)
	}
	cleanRoot := path.Clean(root)
	candidate := userPath
	if userPath == "" {
		candidate = cleanRoot
	} else if !path.IsAbs(candidate) {
		candidate = path.Join(cleanRoot, candidate)
	}
	candidate = path.Clean(candidate)
	grant, ok := matchGrantPOSIX(policy.Grants, candidate)
	if !ok {
		return "", fmt.Errorf("path %q is not allowed", userPath)
	}
	if err := grant.allow(access, userPath); err != nil {
		return "", err
	}
	return candidate, nil
}

func ResolveOS(root, userPath string, policy *PathPolicy, access PathAccess) (string, error) {
	if policy == nil {
		return ResolveUnderRoot(root, userPath)
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	var candidate string
	if strings.TrimSpace(userPath) == "" {
		candidate = cleanRoot
	} else if filepath.IsAbs(userPath) {
		candidate = filepath.Clean(userPath)
	} else {
		candidate = filepath.Clean(filepath.Join(cleanRoot, userPath))
	}
	resolved, err := evalExistingPrefix(candidate)
	if err != nil {
		return "", err
	}
	grant, ok := matchGrantOS(policy.Grants, resolved)
	if !ok {
		return "", fmt.Errorf("path %q is not allowed", userPath)
	}
	if err := grant.allow(access, userPath); err != nil {
		return "", err
	}
	return candidate, nil
}

func (g PathGrant) allow(access PathAccess, userPath string) error {
	switch access {
	case PathRead:
		if !g.Read {
			return fmt.Errorf("path %q is not readable", userPath)
		}
	case PathWrite:
		if !g.Write {
			return fmt.Errorf("path %q is not writable", userPath)
		}
	case PathExec:
		if !g.Exec {
			return fmt.Errorf("path %q is not executable", userPath)
		}
	}
	return nil
}

func matchGrantPOSIX(grants []PathGrant, candidate string) (PathGrant, bool) {
	bestLen := -1
	var best PathGrant
	found := false
	for _, grant := range grants {
		root := path.Clean(strings.TrimSpace(grant.Path))
		if root == "" || !posixContained(root, candidate) {
			continue
		}
		if len(root) > bestLen {
			bestLen = len(root)
			best = grant
			found = true
			continue
		}
		if len(root) == bestLen {
			best.Read = best.Read && grant.Read
			best.Write = best.Write && grant.Write
			best.Exec = best.Exec && grant.Exec
		}
	}
	return best, found
}

func matchGrantOS(grants []PathGrant, candidate string) (PathGrant, bool) {
	bestLen := -1
	var best PathGrant
	found := false
	for _, grant := range grants {
		root := filepath.Clean(strings.TrimSpace(grant.Path))
		if root == "" || !osContained(root, candidate) {
			continue
		}
		if len(root) > bestLen {
			bestLen = len(root)
			best = grant
			found = true
			continue
		}
		if len(root) == bestLen {
			best.Read = best.Read && grant.Read
			best.Write = best.Write && grant.Write
			best.Exec = best.Exec && grant.Exec
		}
	}
	return best, found
}

func osContained(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
