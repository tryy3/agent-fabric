#!/usr/bin/env bash
# Point this clone at the repo-managed hooks under .githooks/ (once per clone).
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

chmod +x .githooks/pre-commit
git config --local core.hooksPath .githooks
echo "Installed git hooks from .githooks/ (core.hooksPath)."
echo "pre-commit will dart-format staged files under client/."
