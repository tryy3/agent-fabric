package sandboxcore

import (
	"path/filepath"
	"testing"
)

func TestCheckAccessPOSIXEscape(t *testing.T) {
	_, v := CheckAccessPOSIX("/workspace", "/etc/passwd", nil, PathRead)
	if v == nil || v.Code != AccessEscape {
		t.Fatalf("violation = %#v", v)
	}
}

func TestCheckAccessPOSIXAllowed(t *testing.T) {
	resolved, v := CheckAccessPOSIX("/workspace", "a.txt", nil, PathRead)
	if v != nil {
		t.Fatalf("unexpected violation: %#v", v)
	}
	if resolved != "/workspace/a.txt" {
		t.Fatalf("resolved = %q", resolved)
	}
}

func TestCheckAccessPOSIXNotAllowedWithPolicy(t *testing.T) {
	policy := &PathPolicy{Grants: []PathGrant{
		{Path: "/workspace", Read: true, Write: true},
	}}
	_, v := CheckAccessPOSIX("/workspace", "/cache/x", policy, PathRead)
	if v == nil || v.Code != AccessNotAllowed {
		t.Fatalf("violation = %#v", v)
	}
}

func TestMergePathPolicyAndGrant(t *testing.T) {
	base := &PathPolicy{Grants: []PathGrant{{Path: "/workspace", Read: true, Write: true}}}
	extra := GrantForResolved("/cache", PathWrite)
	merged := MergePathPolicy(base, extra)
	if len(merged.Grants) != 2 {
		t.Fatalf("grants = %+v", merged.Grants)
	}
	got, v := CheckAccessPOSIX("/workspace", "/cache/blob", merged, PathWrite)
	if v != nil {
		t.Fatal(v)
	}
	if got != "/cache/blob" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckAccessOSExtraPath(t *testing.T) {
	root := t.TempDir()
	extra := t.TempDir()
	policy := &PathPolicy{Grants: []PathGrant{
		{Path: root, Read: true, Write: true},
		{Path: extra, Read: true, Write: true},
	}}
	target := filepath.Join(extra, "a.txt")
	got, v := CheckAccessOS(root, target, policy, PathWrite)
	if v != nil {
		t.Fatal(v)
	}
	if got != target {
		t.Fatalf("got %q", got)
	}
}
