package scrub

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PromptScrubCLI runs the Nano Collective prompt-scrub CLI one-way (no session map kept).
type PromptScrubCLI struct {
	// Bin is the executable path. Empty uses PROMPT_SCRUB_BIN or "prompt-scrub".
	Bin string
	// Timeout caps a single scrub invocation. Zero uses 15s.
	Timeout time.Duration
}

func (p PromptScrubCLI) bin() string {
	if p.Bin != "" {
		return p.Bin
	}
	if v := strings.TrimSpace(os.Getenv("PROMPT_SCRUB_BIN")); v != "" {
		return v
	}
	return "prompt-scrub"
}

func (p PromptScrubCLI) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return 15 * time.Second
}

// resolveBin returns an executable path safe to exec.
//
// npm's global bin is a symlink. prompt-scrub's CLI only parses argv when
// process.argv[1] === fileURLToPath(import.meta.url); invoking via the symlink
// makes that check fail, so the process exits 0 with empty stdout. Resolving
// the symlink makes argv[1] match the real module path.
func resolveBin(bin string) (string, error) {
	path := bin
	if !filepath.IsAbs(path) {
		looked, err := exec.LookPath(path)
		if err != nil {
			return "", err
		}
		path = looked
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// Scrub pipes content through `prompt-scrub scrub -q`. Session ids on stderr are discarded.
func (p PromptScrubCLI) Scrub(ctx context.Context, content string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	bin, err := resolveBin(p.bin())
	if err != nil {
		return "", fmt.Errorf("prompt-scrub: resolve bin: %w", err)
	}

	cmd := exec.CommandContext(runCtx, bin, "scrub", "-q")
	cmd.Stdin = strings.NewReader(content)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("prompt-scrub: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	out := stdout.String()
	// npm-symlink no-op and similar silent failures exit 0 with empty stdout.
	if content != "" && out == "" {
		return "", fmt.Errorf("prompt-scrub: empty scrub output for non-empty input (%s)", strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// NewBodyScrubberFromEnv returns PromptScrubCLI when the binary is available, else Identity.
func NewBodyScrubberFromEnv() ContentScrubber {
	bin := strings.TrimSpace(os.Getenv("PROMPT_SCRUB_BIN"))
	if bin == "" {
		bin = "prompt-scrub"
	}
	resolved, err := resolveBin(bin)
	if err != nil {
		return Identity{}
	}
	return PromptScrubCLI{Bin: resolved}
}
