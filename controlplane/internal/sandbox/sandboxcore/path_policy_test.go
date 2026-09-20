package sandboxcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePOSIXLegacyJailsToWorkspaceRoot(t *testing.T) {
	got, err := ResolvePOSIX("/workspace", "a.txt", nil, PathRead)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/a.txt" {
		t.Fatalf("got %q", got)
	}
	if _, err := ResolvePOSIX("/workspace", "/etc/passwd", nil, PathRead); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolvePOSIXWhitelistAndReadonly(t *testing.T) {
	policy := &PathPolicy{Grants: []PathGrant{
		{Path: "/workspace", Read: true, Write: false},
		{Path: "/cache", Read: true, Write: true},
	}}
	got, err := ResolvePOSIX("/workspace", "notes.txt", policy, PathRead)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/notes.txt" {
		t.Fatalf("got %q", got)
	}
	if _, err := ResolvePOSIX("/workspace", "notes.txt", policy, PathWrite); err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("readonly err = %v", err)
	}
	got, err = ResolvePOSIX("/workspace", "/cache/blob", policy, PathWrite)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/cache/blob" {
		t.Fatalf("extra path = %q", got)
	}
	if _, err := ResolvePOSIX("/workspace", "/etc/passwd", policy, PathRead); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("outside err = %v", err)
	}
	if _, err := ResolvePOSIX("/workspace", "../etc/passwd", policy, PathRead); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("dotdot err = %v", err)
	}
}

func TestResolvePOSIXMostSpecificDenyWins(t *testing.T) {
	policy := &PathPolicy{Grants: []PathGrant{
		{Path: "/workspace", Read: true, Write: true},
		{Path: "/workspace/ro", Read: true, Write: false},
	}}
	if _, err := ResolvePOSIX("/workspace", "/workspace/ro/x", policy, PathWrite); err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("nested readonly err = %v", err)
	}
	got, err := ResolvePOSIX("/workspace", "/workspace/ok.txt", policy, PathWrite)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/ok.txt" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveOSAllowsExtraHostPath(t *testing.T) {
	root := t.TempDir()
	extra := t.TempDir()
	policy := &PathPolicy{Grants: []PathGrant{
		{Path: root, Read: true, Write: true},
		{Path: extra, Read: true, Write: true},
	}}
	got, err := ResolveOS(root, filepath.Join(extra, "a.txt"), policy, PathWrite)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(extra, "a.txt") {
		t.Fatalf("got %q", got)
	}
	outside := t.TempDir()
	if _, err := ResolveOS(root, filepath.Join(outside, "secret.txt"), policy, PathRead); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("outside err = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveOS(root, "ok.txt", policy, PathRead)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "ok.txt") {
		t.Fatalf("relative = %q", got)
	}
}
