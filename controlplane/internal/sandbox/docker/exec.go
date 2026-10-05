package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os/exec"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

type CommandResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
}

// CappedRunner is an optional CommandRunner extension that bounds the
// captured stdout and stderr while the command runs.
type CappedRunner interface {
	RunCapped(
		ctx context.Context,
		name string,
		args []string,
		stdin []byte,
		maxOutput int,
	) (CommandResult, error)
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

func (r OSRunner) Run(
	ctx context.Context,
	name string,
	args []string,
	stdin []byte,
) (CommandResult, error) {
	return r.RunCapped(ctx, name, args, stdin, 0)
}

func (OSRunner) RunCapped(
	ctx context.Context,
	name string,
	args []string,
	stdin []byte,
	maxOutput int,
) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = bytes.NewReader(stdin)
	stdout := &sandboxcore.CappedBuffer{Max: maxOutput}
	stderr := &sandboxcore.CappedBuffer{Max: maxOutput}
	command.Stdout = stdout
	command.Stderr = stderr

	err := command.Run()
	result := CommandResult{
		Stdout:          stdout.Bytes(),
		Stderr:          stderr.Bytes(),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
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
	containerID string
	bin         string
	projectRoot string
	runner      CommandRunner
	touch       func()
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

	workDir, err := containerWorkDir(e.projectRoot, req.WorkDir)
	if err != nil {
		return sandboxcore.ExecResult{}, err
	}
	args := []string{"exec"}
	if len(req.Stdin) > 0 {
		args = append(args, "-i")
	}
	args = append(args, "-w", workDir, e.containerID)
	pidFile := ""
	if req.KillOnCancel {
		// Cancelling `docker exec` only ends the client; the command keeps
		// running in the container. Record the pid (exec keeps it) so a
		// cancel can kill the command itself.
		pidFile = "/tmp/.agent-fabric-exec-" + randomID()
		args = append(args, "sh", "-c", `echo $$ > "$0"; exec "$@"`, pidFile)
	}
	args = append(args, req.Cmd...)
	if e.touch != nil {
		e.touch()
	}
	var result CommandResult
	if capped, ok := e.runner.(CappedRunner); ok && req.MaxOutputBytes > 0 {
		result, err = capped.RunCapped(ctx, e.bin, args, req.Stdin, req.MaxOutputBytes)
	} else {
		result, err = e.runner.Run(ctx, e.bin, args, req.Stdin)
		var outCut, errCut bool
		result.Stdout, outCut = sandboxcore.CapBytes(result.Stdout, req.MaxOutputBytes)
		result.Stderr, errCut = sandboxcore.CapBytes(result.Stderr, req.MaxOutputBytes)
		result.StdoutTruncated = result.StdoutTruncated || outCut
		result.StderrTruncated = result.StderrTruncated || errCut
	}
	if pidFile != "" && ctx.Err() != nil {
		e.killInContainer(pidFile)
	}
	return sandboxcore.ExecResult{
		ExitCode:        result.ExitCode,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		StdoutTruncated: result.StdoutTruncated,
		StderrTruncated: result.StderrTruncated,
	}, err
}

// killInContainer ends the command recorded in pidFile and its direct
// children. It runs on a context detached from the cancelled one.
func (e *containerExecutor) killInContainer(pidFile string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const script = `p=$(cat "$0" 2>/dev/null) || exit 0; pkill -KILL -P "$p" 2>/dev/null; kill -KILL "$p" 2>/dev/null; rm -f "$0"`
	_, _ = e.runner.Run(ctx, e.bin, []string{"exec", e.containerID, "sh", "-c", script, pidFile}, nil)
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func containerWorkDir(root, requested string) (string, error) {
	return sandboxcore.ContainUnderRootPOSIX(root, requested)
}
