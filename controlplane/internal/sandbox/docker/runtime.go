package docker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

const imageTagPrefix = "agent-fabric-sandbox:"

type imageRunner interface {
	CombinedOutput(
		ctx context.Context,
		name string,
		args ...string,
	) ([]byte, error)
}

func ResolveImage(
	ctx context.Context,
	runner imageRunner,
	bin string,
	opts sandboxcore.DockerOptions,
) (string, error) {
	if opts.Image != "" && opts.Dockerfile != "" {
		return "", errors.New("docker requires exactly one of image or dockerfile")
	}
	if opts.Image != "" {
		return opts.Image, nil
	}
	if opts.Dockerfile == "" {
		return "", errors.New("docker requires exactly one of image or dockerfile")
	}

	contents, err := os.ReadFile(opts.Dockerfile)
	if err != nil {
		return "", fmt.Errorf("read Dockerfile: %w", err)
	}
	tag := fmt.Sprintf("%s%x", imageTagPrefix, sha256.Sum256(contents))
	output, err := runner.CombinedOutput(ctx, bin, "images", "-q", tag)
	if err != nil {
		return "", commandError("inspect sandbox image", output, err)
	}
	if strings.TrimSpace(string(output)) != "" {
		return tag, nil
	}

	buildContext := opts.BuildContext
	if buildContext == "" {
		buildContext = filepath.Dir(opts.Dockerfile)
	}
	output, err = runner.CombinedOutput(
		ctx,
		bin,
		"build",
		"-t",
		tag,
		"-f",
		opts.Dockerfile,
		buildContext,
	)
	if err != nil {
		return "", commandError("build sandbox image", output, err)
	}
	return tag, nil
}

func commandError(action string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}
