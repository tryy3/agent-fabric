package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type CommandResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

type CommandRunner interface {
	Run(
		ctx context.Context,
		name string,
		args []string,
		stdin []byte,
	) (CommandResult, error)
}

type OSRunner struct{}

func (OSRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (OSRunner) CombinedOutput(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (OSRunner) Run(
	ctx context.Context,
	name string,
	args []string,
	stdin []byte,
) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	result := CommandResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	if exitErr != nil {
		return result, nil
	}
	return result, err
}

type containerExecutor struct {
	containerID   string
	bin           string
	workspaceRoot string
	runner        CommandRunner
	touch         func()
}

func (e *containerExecutor) Run(
	ctx context.Context,
	req sandboxcore.ExecRequest,
) (sandboxcore.ExecResult, error) {
	if len(req.Cmd) == 0 {
		return sandboxcore.ExecResult{}, errors.New("command is required")
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	workDir, err := containerWorkDir(e.workspaceRoot, req.WorkDir)
	if err != nil {
		return sandboxcore.ExecResult{}, err
	}
	args := []string{"exec"}
	if len(req.Stdin) > 0 {
		args = append(args, "-i")
	}
	args = append(args, "-w", workDir, e.containerID)
	args = append(args, req.Cmd...)
	if e.touch != nil {
		e.touch()
	}
	result, err := e.runner.Run(ctx, e.bin, args, req.Stdin)
	return sandboxcore.ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, err
}

func containerWorkDir(root, requested string) (string, error) {
	if !path.IsAbs(root) {
		return "", fmt.Errorf("workspace root must be absolute: %q", root)
	}
	cleanRoot := path.Clean(root)
	if requested == "" {
		return cleanRoot, nil
	}
	candidate := requested
	if !path.IsAbs(candidate) {
		candidate = path.Join(cleanRoot, candidate)
	}
	candidate = path.Clean(candidate)
	prefix := cleanRoot
	if prefix != "/" {
		prefix += "/"
	}
	if candidate != cleanRoot && !strings.HasPrefix(candidate, prefix) {
		return "", fmt.Errorf(
			"work directory %q escapes workspace root %q",
			requested,
			cleanRoot,
		)
	}
	return candidate, nil
}
