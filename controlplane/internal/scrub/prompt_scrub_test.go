package scrub_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tryy3/agent-fabric/internal/scrub"
)

func TestPromptScrubCLIResolvesSymlinkBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink CLI fixture is unix-oriented")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real-scrub")
	// Echo stdin; our wrapper must EvalSymlinks so exec uses this real file.
	if err := os.WriteFile(real, []byte("#!/bin/sh\ncat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "prompt-scrub")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	cli := scrub.PromptScrubCLI{Bin: link}
	got, err := cli.Scrub(context.Background(), `{"hi":"alice@acme.com"}`)
	if err != nil {
		t.Fatalf("Scrub: %v", err)
	}
	if got != `{"hi":"alice@acme.com"}` {
		t.Fatalf("got %q", got)
	}
}

func TestPromptScrubCLIRejectsEmptyStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "empty-scrub")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cli := scrub.PromptScrubCLI{Bin: bin}
	_, err := cli.Scrub(context.Background(), "non-empty")
	if err == nil {
		t.Fatal("expected error for empty scrub output")
	}
}

func TestNewBodyScrubberFromEnvMissingFallsBack(t *testing.T) {
	t.Setenv("PROMPT_SCRUB_BIN", filepath.Join(t.TempDir(), "no-such-prompt-scrub"))
	s := scrub.NewBodyScrubberFromEnv()
	if _, ok := s.(scrub.Identity); !ok {
		t.Fatalf("got %T, want Identity", s)
	}
}

func TestPromptScrubCLIIntegration(t *testing.T) {
	bin := os.Getenv("PROMPT_SCRUB_BIN")
	if bin == "" {
		bin = "prompt-scrub"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("prompt-scrub not on PATH")
	}
	cli := scrub.PromptScrubCLI{Bin: bin}
	in := "My email is alice@acme.com"
	got, err := cli.Scrub(context.Background(), in)
	if err != nil {
		t.Fatalf("Scrub: %v", err)
	}
	if got == "" {
		t.Fatal("empty scrub output")
	}
	if got == in {
		// May be a no-op Identity path if detectors miss; still must be non-empty.
		t.Logf("unchanged output (no entities): %q", got)
	}
}
