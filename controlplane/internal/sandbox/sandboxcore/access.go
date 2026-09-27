package sandboxcore

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// AccessViolationCode classifies why a path failed a preflight check.
type AccessViolationCode string

const (
	AccessOK          AccessViolationCode = ""
	AccessEscape      AccessViolationCode = "escape"
	AccessNotAllowed  AccessViolationCode = "not_allowed"
	AccessNotReadable AccessViolationCode = "not_readable"
	AccessNotWritable AccessViolationCode = "not_writable"
	AccessNotExec     AccessViolationCode = "not_executable"
)

// AccessViolation is a structured path-policy failure for gate preflight.
type AccessViolation struct {
	Code    AccessViolationCode
	Path    string
	Access  PathAccess
	Message string
}

func (v *AccessViolation) Error() string {
	if v == nil {
		return ""
	}
	return v.Message
}

// CheckAccessPOSIX preflights a path against root/policy without I/O.
func CheckAccessPOSIX(root, userPath string, policy *PathPolicy, access PathAccess) (string, *AccessViolation) {
	resolved, err := ResolvePOSIX(root, userPath, policy, access)
	if err == nil {
		return resolved, nil
	}
	return "", classifyAccessError(userPath, access, err)
}

// CheckAccessOS preflights a host path against root/policy (symlink-aware).
func CheckAccessOS(root, userPath string, policy *PathPolicy, access PathAccess) (string, *AccessViolation) {
	resolved, err := ResolveOS(root, userPath, policy, access)
	if err == nil {
		return resolved, nil
	}
	return "", classifyAccessError(userPath, access, err)
}

// MergePathPolicy returns a copy of base with extra grants appended.
func MergePathPolicy(base *PathPolicy, extra ...PathGrant) *PathPolicy {
	var grants []PathGrant
	if base != nil {
		grants = append(grants, base.Grants...)
	}
	grants = append(grants, extra...)
	if len(grants) == 0 {
		return nil
	}
	return &PathPolicy{Grants: grants}
}

// GrantForResolved builds a grant covering resolvedPath for the given access.
func GrantForResolved(resolvedPath string, access PathAccess) PathGrant {
	g := PathGrant{Path: resolvedPath}
	switch access {
	case PathRead:
		g.Read = true
	case PathWrite:
		g.Read = true
		g.Write = true
	case PathExec:
		g.Read = true
		g.Exec = true
	}
	return g
}

// CandidatePOSIX returns the cleaned absolute candidate path under root without policy checks.
func CandidatePOSIX(root, userPath string) string {
	if !path.IsAbs(root) {
		return ""
	}
	cleanRoot := path.Clean(root)
	if userPath == "" {
		return cleanRoot
	}
	candidate := userPath
	if !path.IsAbs(candidate) {
		candidate = path.Join(cleanRoot, candidate)
	}
	return path.Clean(candidate)
}

// CandidateOS returns the cleaned absolute candidate path under root without policy checks.
func CandidateOS(root, userPath string) string {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return ""
	}
	if strings.TrimSpace(userPath) == "" {
		return cleanRoot
	}
	if filepath.IsAbs(userPath) {
		return filepath.Clean(userPath)
	}
	return filepath.Clean(filepath.Join(cleanRoot, userPath))
}

func classifyAccessError(userPath string, access PathAccess, err error) *AccessViolation {
	msg := err.Error()
	code := AccessNotAllowed
	switch {
	case strings.Contains(msg, "escapes workspace root"):
		code = AccessEscape
	case strings.Contains(msg, "not readable"):
		code = AccessNotReadable
	case strings.Contains(msg, "not writable"):
		code = AccessNotWritable
	case strings.Contains(msg, "not executable"):
		code = AccessNotExec
	case strings.Contains(msg, "not allowed"):
		code = AccessNotAllowed
	default:
		code = AccessNotAllowed
		msg = fmt.Sprintf("path %q is not allowed: %v", userPath, err)
	}
	return &AccessViolation{
		Code:    code,
		Path:    userPath,
		Access:  access,
		Message: msg,
	}
}
