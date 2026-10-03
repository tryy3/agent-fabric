package udiff_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file/udiff"
)

func parseOne(t *testing.T, diff string) udiff.File {
	t.Helper()
	files, err := udiff.Parse(diff)
	if err != nil || len(files) != 1 {
		t.Fatalf("Parse = %v, %v", files, err)
	}
	return files[0]
}

func TestParseHeadersAndPrefixes(t *testing.T) {
	f := parseOne(t, "diff --git a/x.go b/x.go\nindex 1..2\n--- a/x.go\t2026-01-01\n+++ b/x.go\n@@ -1 +1 @@ func main\n-a\n+b\n")
	if f.Path != "x.go" || f.Create || len(f.Hunks) != 1 || f.Hunks[0].OldCount != 1 {
		t.Fatalf("file = %+v", f)
	}
	plain := parseOne(t, "--- x.go\n+++ x.go\n@@ -1 +1 @@\n-a\n+b\n")
	if plain.Path != "x.go" {
		t.Fatalf("plain path = %q", plain.Path)
	}
	created := parseOne(t, "--- /dev/null\n+++ b/n.txt\n@@ -0,0 +1 @@\n+x\n")
	if !created.Create || created.Path != "n.txt" {
		t.Fatalf("created = %+v", created)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]error{
		"--- a/x\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n": udiff.ErrUnsupported,
		"--- a/x\n+++ b/y\n@@ -1 +1 @@\n-a\n+b\n":     udiff.ErrUnsupported,
		"rename from a\nrename to b\n":                udiff.ErrUnsupported,
	}
	for diff, want := range cases {
		if _, err := udiff.Parse(diff); !errors.Is(err, want) {
			t.Errorf("Parse(%q) = %v, want %v", diff, err, want)
		}
	}
	for _, diff := range []string{
		"",
		"just prose",
		"--- a/x\n",
		"--- a/x\n+++ b/x\n",
		"--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n-a\n+b\n", // declares 2 lines, has 1
		"--- a/x\n+++ b/x\n@@ nonsense @@\n",
		"--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-b\n+c\n", // duplicate file
	} {
		if _, err := udiff.Parse(diff); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", diff)
		}
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name, original, diff, want string
	}{
		{"replace", "a\nb\nc\n", "--- a/f\n+++ b/f\n@@ -2 +2 @@\n-b\n+B\n", "a\nB\nc\n"},
		{"context", "a\nb\nc\n", "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n", "a\nB\nc\n"},
		{"two hunks", "1\n2\n3\n4\n5\n", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-1\n+one\n@@ -5 +5 @@\n-5\n+five\n", "one\n2\n3\n4\nfive\n"},
		{"insert after line", "a\nc\n", "--- a/f\n+++ b/f\n@@ -1,0 +2 @@\n+b\n", "a\nb\nc\n"},
		{"delete lines", "a\nb\nc\n", "--- a/f\n+++ b/f\n@@ -2,2 +1,0 @@\n-b\n-c\n", "a\n"},
		{"stripped empty context", "a\n\nb\n", "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n\n-b\n+B\n", "a\n\nB\n"},
		{"preserves missing final newline", "a\nb", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+A\n", "A\nb"},
		{"adds final newline", "a", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+a\n", "a\n"},
		{"removes final newline", "a\n", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+a\n\\ No newline at end of file\n", "a"},
		{"create", "", "--- /dev/null\n+++ b/f\n@@ -0,0 +1,2 @@\n+x\n+y\n", "x\ny\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := udiff.Apply(tc.original, parseOne(t, tc.diff))
			if err != nil || got != tc.want {
				t.Fatalf("Apply = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestApplyMismatch(t *testing.T) {
	for name, diff := range map[string]string{
		"wrong text":     "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-zzz\n+y\n",
		"wrong position": "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-c\n+y\n",
		"past eof":       "--- a/f\n+++ b/f\n@@ -9 +9 @@\n-a\n+y\n",
		"context":        "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\n-zzz\n+y\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := udiff.Apply("a\nb\nc\n", parseOne(t, diff))
			if !errors.Is(err, udiff.ErrMismatch) {
				t.Fatalf("err = %v, want ErrMismatch", err)
			}
		})
	}
	t.Run("overlap", func(t *testing.T) {
		d := "--- a/f\n+++ b/f\n@@ -2 +2 @@\n-b\n+B\n@@ -1 +1 @@\n-a\n+A\n"
		if _, err := udiff.Apply("a\nb\nc\n", parseOne(t, d)); err == nil || !strings.Contains(err.Error(), "overlaps") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("create over content", func(t *testing.T) {
		d := parseOne(t, "--- /dev/null\n+++ b/f\n@@ -0,0 +1 @@\n+x\n")
		if _, err := udiff.Apply("existing\n", d); !errors.Is(err, udiff.ErrMismatch) {
			t.Fatalf("err = %v", err)
		}
	})
}
