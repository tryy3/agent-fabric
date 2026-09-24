package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type environment struct {
	root string
	fs   sandboxcore.FS
	exec sandboxcore.Executor
}

func New(root string, policy *sandboxcore.PathPolicy) (sandboxcore.Environment, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local workspace root is required")
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %q is not a directory", absoluteRoot)
	}

	return &environment{
		root: absoluteRoot,
		fs:   &localFS{root: absoluteRoot, policy: policy},
		exec: &localExecutor{root: absoluteRoot},
	}, nil
}

func (e *environment) ID() string {
	return e.root
}

func (e *environment) Caps() sandboxcore.Capabilities {
	return sandboxcore.Capabilities{FS: true, Exec: true}
}

func (e *environment) FS() (sandboxcore.FS, bool) {
	return e.fs, true
}

func (e *environment) Exec() (sandboxcore.Executor, bool) {
	return e.exec, true
}

func (e *environment) Close(context.Context) error {
	return nil
}

type localExecutor struct {
	root string
}

func (e *localExecutor) Run(ctx context.Context, req sandboxcore.ExecRequest) (sandboxcore.ExecResult, error) {
	if len(req.Cmd) == 0 {
		return sandboxcore.ExecResult{}, fmt.Errorf("command is required")
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	workDir, err := sandboxcore.ResolveUnderRoot(e.root, req.WorkDir)
	if err != nil {
		return sandboxcore.ExecResult{}, err
	}
	command := exec.CommandContext(ctx, req.Cmd[0], req.Cmd[1:]...)
	command.Dir = workDir
	command.Stdin = bytes.NewReader(req.Stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()
	result := sandboxcore.ExecResult{
		ExitCode: 0,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
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
