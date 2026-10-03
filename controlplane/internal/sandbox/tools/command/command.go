// Package command implements the non-interactive run_command environment tool.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// Name is the tool name.
const Name = "run_command"

// Args is the decoded run_command input. The Gate decodes the same shape.
type Args struct {
	Command        []string `json:"command"`
	Cwd            string   `json:"cwd,omitempty"`
	Stdin          string   `json:"stdin,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

// Decode parses and validates raw tool arguments.
func Decode(raw json.RawMessage) (Args, error) {
	var args Args
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return Args{}, fmt.Errorf("decode run_command arguments: %w", err)
	}
	if len(args.Command) == 0 || strings.TrimSpace(args.Command[0]) == "" {
		return Args{}, errors.New("run_command command is required (argv array, first element is the program)")
	}
	if len(args.Command) > MaxArgs {
		return Args{}, fmt.Errorf("run_command command has more than %d arguments", MaxArgs)
	}
	for _, a := range args.Command {
		if len(a) > MaxArgBytes {
			return Args{}, fmt.Errorf("run_command argument exceeds %d bytes", MaxArgBytes)
		}
		if strings.ContainsRune(a, 0) {
			return Args{}, errors.New("run_command arguments must not contain NUL")
		}
	}
	if len(args.Stdin) > MaxStdinBytes {
		return Args{}, fmt.Errorf("run_command stdin exceeds %d bytes", MaxStdinBytes)
	}
	if args.TimeoutSeconds < 0 {
		return Args{}, errors.New("run_command timeout_seconds must not be negative")
	}
	return args, nil
}

// Timeout returns the effective, clamped timeout.
func (a Args) Timeout() time.Duration {
	if a.TimeoutSeconds <= 0 {
		return DefaultTimeout
	}
	return min(time.Duration(a.TimeoutSeconds)*time.Second, MaxTimeout)
}

// Tools returns the run_command tool definition.
func Tools() []sandbox.Tool {
	return []sandbox.Tool{{
		Name: Name,
		Description: "Run one non-interactive command in the project environment (no shell: pass an argv array such as " +
			`["npm","test"]). Returns exit_code, stdout, stderr and duration_ms; a non-zero exit code is a normal result. ` +
			"Output is truncated past 64 KiB per stream. Read-only commands run immediately; others may ask the user for approval.",
		Parameters: sandbox.Parameters{
			Properties: map[string]sandbox.Property{
				"command": {
					Type:        "array",
					Description: "Program and arguments, e.g. [\"go\",\"test\",\"./...\"]. Not a shell string.",
					Items:       &sandbox.Property{Type: "string"},
				},
				"cwd":             {Type: "string", Description: "Working directory under the workspace root (default: the root)"},
				"stdin":           {Type: "string", Description: "Optional text piped to the command's stdin"},
				"timeout_seconds": {Type: "integer", Description: "Timeout in seconds (default 120, max 900)"},
			},
			Required: []string{"command"},
		},
		Requires: sandbox.Capabilities{Exec: true},
		Run:      run,
	}}
}

type result struct {
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	DurationMs      int64  `json:"duration_ms"`
	TimedOut        bool   `json:"timed_out"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
}

func run(ctx context.Context, env sandbox.Environment, raw json.RawMessage) (string, error) {
	args, err := Decode(raw)
	if err != nil {
		return fail(codeInvalidArgs, "%v", err)
	}
	executor, ok := env.Exec()
	if !ok {
		return fail(codeIO, "command execution is unavailable")
	}
	start := time.Now()
	res, runErr := executor.Run(ctx, sandbox.ExecRequest{
		Cmd:            args.Command,
		WorkDir:        args.Cwd,
		Stdin:          []byte(args.Stdin),
		Timeout:        args.Timeout(),
		MaxOutputBytes: MaxOutputBytes,
	})
	timedOut := false
	if runErr != nil {
		switch {
		case ctx.Err() != nil:
			// Stop / session cancel: abort the turn, not a model-visible result.
			return "", ctx.Err()
		case errors.Is(runErr, context.DeadlineExceeded):
			timedOut = true
		default:
			return fail(codeIO, "run %q: %v", args.Command[0], runErr)
		}
	}
	out := result{
		ExitCode:        res.ExitCode,
		Stdout:          text(res.Stdout),
		Stderr:          text(res.Stderr),
		DurationMs:      time.Since(start).Milliseconds(),
		TimedOut:        timedOut,
		StdoutTruncated: res.StdoutTruncated,
		StderrTruncated: res.StderrTruncated,
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encode tool result: %w", err)
	}
	return string(encoded), nil
}

// text returns valid UTF-8, dropping a rune cut by the output cap.
func text(b []byte) string {
	return strings.ToValidUTF8(string(b), "�")
}

func fail(code, format string, args ...any) (string, error) {
	b, err := json.Marshal(struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}{Error: fmt.Sprintf(format, args...), Code: code})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
