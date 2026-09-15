# Task 7 Report: Docker Image Resolve + Open

## Status

Complete. `sandbox.Open` now creates Docker/Podman environments backed by the
container manager, container execution, and `execfs`.

## Implementation

- Added Dockerfile image resolution using a SHA-256 content tag, image reuse via
  `images -q`, and builds via `build -t ... -f ...`.
- Added OS command execution with stdout, stderr, exit code, stdin, timeout, and
  container workspace handling.
- Added Docker environment capabilities, exec-backed filesystem access, scoped
  container acquisition, activity touches, and idempotent `Manager.Done` close.
- Added shared and session scope keys, including required session ID validation.
- Moved `execfs` to `sandboxcore` contracts to avoid the sandbox/docker import
  cycle.

Implementation commit: `f1ffb08`

## TDD and Verification

The initial Docker package test run failed on missing `ResolveImage`, `NewEnv`,
and `CommandResult`, as expected.

Final verification:

```text
nix develop -c go -C controlplane test ./internal/sandbox/... ./internal/sandboxconfig/... -count=1
```

All packages passed. IDE lint diagnostics and `git diff --check` were clean.

## Concerns

- No real container runtime was invoked; integration coverage remains Task 8.
- `Close` only releases the manager reference. It deliberately does not remove
  containers; removal remains the idle reaper's responsibility.
