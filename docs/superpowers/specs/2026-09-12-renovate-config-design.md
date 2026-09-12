# Renovate configuration

**Date:** 2026-09-12  
**Status:** approved for planning  
**Related:** [PR #4](https://github.com/tryy3/agent-fabric/pull/4) (Renovate onboarding)

## Problem

Mend Renovate opened an onboarding PR with a minimal `renovate.json` (`config:recommended` only). The repo is a small monorepo (Go control plane, Flutter client, Nix flake). Default config does not enable Nix, does not group non-major updates for a balanced PR volume, and does not match our conventional commit style explicitly.

## Goals

- Activate Renovate with a config suited to this project’s package managers.
- Keep dependency PR noise balanced: group patch/minor; leave majors separate.
- Update Go and Flutter/Dart as soon as updates appear.
- Update Nix (`flake.lock`) on a weekly cadence only.
- Use conventional commits and a `dependencies` label.
- Keep Dependency Dashboard enabled.

## Non-goals

- Automerge (no GitHub Actions CI yet).
- Custom managers for `skills-lock.json` or other agent skill locks.
- Docker / npm / GitHub Actions managers (none present today).
- Changing Mend org/app settings outside `renovate.json`.

## Approach

**Chosen:** Preset-heavy config — extend `config:recommended` and `group:allNonMajor`, enable `nix`, schedule only nix updates weekly, set semantic commit type and labels.

**Rejected:**

- Fully explicit `packageRules` for every manager — more maintenance for the same behavior at this repo size.
- Minimal recommended + Nix only — noisier minor/patch PRs than the balanced preference.

## Managers in scope

| Manager | Paths | Cadence |
|---------|--------|---------|
| `gomod` | `controlplane/go.mod`, `go.sum` | Immediate |
| `pub` | `client/pubspec.yaml`, lockfile | Immediate |
| `nix` | `flake.lock` | Weekly (`before 6am on monday`) |

## Grouping & PRs

- Non-major updates across packages: grouped via `group:allNonMajor`.
- Major updates: separate PRs (default behavior with that preset).
- Lockfile maintenance enabled so `go.sum` / pub lock do not drift silently.
- Range strategy: Renovate defaults (pub keeps `^` ranges; Go modules remain version-pinned as today).

## Commits & labels

- Semantic commits: `chore(deps): …` for dependency updates.
- Label all Renovate PRs with `dependencies`.
- Dependency Dashboard remains on (from recommended).

## Safety

- Automerge: disabled.
- No vulnerability automerge or forced rebases beyond Renovate defaults.
- Revisit automerge after CI exists for Go and Flutter.

## Deliverable

Update `renovate.json` on the onboarding branch (PR #4) so merging that PR activates Renovate with this policy. No application code changes.

## Success criteria

- After merge, Renovate detects gomod, pub, and nix.
- Non-major Go/pub updates land in grouped PRs; majors are separate.
- Nix/`flake.lock` PRs only appear on the weekly schedule.
- Renovate PR commits use `chore(deps)` and the `dependencies` label.
