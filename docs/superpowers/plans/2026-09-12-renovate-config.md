# Renovate Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the onboarding `renovate.json` on PR #4 with a project-specific Renovate config so Go/pub update immediately (non-majors grouped), Nix/`flake.lock` updates weekly, and PRs use `chore(deps)` + `dependencies`.

**Architecture:** Preset-heavy `renovate.json` — extend `config:recommended` and `group:allNonMajor`, enable the beta `nix` manager, enable lockfile maintenance, and use `packageRules` only to schedule Nix updates weekly. No application code changes.

**Tech Stack:** Mend Renovate (GitHub App), `renovate.json` JSON Schema, managers `gomod` / `pub` / `nix`.

## Global Constraints

- Work on branch `renovate/configure` (PR https://github.com/tryy3/agent-fabric/pull/4).
- Spec: `docs/superpowers/specs/2026-09-12-renovate-config-design.md`.
- Automerge: disabled.
- Do not add custom managers for `skills-lock.json`.
- Do not enable Docker / npm / GitHub Actions managers (none present).
- Nix schedule string: `before 6am on monday` (exact).
- Labels: exactly `dependencies`.
- Commits: conventional `chore(deps): …` via Renovate semantic-commit presets.
- Do not force-push or close the onboarding PR; update `renovate.json` in place and push so Renovate refreshes the PR description on next run.

## File Structure

| Path | Responsibility |
| --- | --- |
| `renovate.json` | Sole Renovate policy file (managers, groups, schedules, labels, commits) |
| `docs/superpowers/specs/2026-09-12-renovate-config-design.md` | Approved design (read-only during implementation) |
| `controlplane/go.mod` | Detected by Renovate (`gomod`) — do not edit |
| `client/pubspec.yaml` | Detected by Renovate (`pub`) — do not edit |
| `flake.nix` / `flake.lock` | Detected after `nix.enabled` — do not edit in this plan |

---

### Task 1: Write and validate `renovate.json`

**Files:**
- Modify: `renovate.json`
- Test: `python` JSON parse + schema key sanity; optional `npx --yes renovate-config-validator` if network available

**Interfaces:**
- Consumes: design decisions in `docs/superpowers/specs/2026-09-12-renovate-config-design.md`
- Produces: root `renovate.json` that Renovate will re-read on next repository job after merge (or after push to the onboarding branch)

- [ ] **Step 1: Confirm branch and current file**

```bash
cd /home/tryy3/src/agent-fabric
git checkout renovate/configure
git status
cat renovate.json
```

Expected: on `renovate/configure`; current file is the minimal onboarding config:

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": [
    "config:recommended"
  ]
}
```

- [ ] **Step 2: Replace `renovate.json` with the project config**

Write the entire file as:

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": [
    "config:recommended",
    "group:allNonMajor",
    ":semanticCommits",
    ":semanticCommitTypeAll(chore)",
    ":semanticCommitScope(deps)"
  ],
  "labels": ["dependencies"],
  "automerge": false,
  "nix": {
    "enabled": true
  },
  "lockFileMaintenance": {
    "enabled": true
  },
  "packageRules": [
    {
      "description": "Nix flake input updates weekly only",
      "matchManagers": ["nix"],
      "schedule": ["before 6am on monday"]
    },
    {
      "description": "Nix flake.lock maintenance weekly only",
      "matchUpdateTypes": ["lockFileMaintenance"],
      "matchFileNames": ["**/flake.lock"],
      "schedule": ["before 6am on monday"]
    }
  ]
}
```

Notes for the implementer:

- `group:allNonMajor` groups patch/minor; majors stay separate PRs.
- `nix.enabled` is required (manager is opt-in / beta).
- Go (`gomod`) and Flutter (`pub`) keep default (immediate) scheduling.
- Global `lockFileMaintenance` covers `go.sum` / pub lock on Renovate’s default lock-maintenance schedule; the second `packageRule` forces `flake.lock` maintenance onto Monday mornings only.
- `automerge: false` is explicit even though false is the default.

- [ ] **Step 3: Validate JSON parses**

```bash
python -c 'import json; json.load(open("renovate.json")); print("ok")'
```

Expected: prints `ok`.

- [ ] **Step 4: Validate against Renovate schema (preferred if network works)**

```bash
npx --yes --package renovate -- renovate-config-validator renovate.json
```

Expected: validator reports the config is valid (no error exit). If `npx` / network is unavailable, skip this step and rely on Step 3 plus a manual key check: `$schema`, `extends`, `labels`, `automerge`, `nix.enabled`, `lockFileMaintenance.enabled`, and both `packageRules` entries present.

- [ ] **Step 5: Commit**

```bash
git add renovate.json
git commit -m "$(cat <<'EOF'
chore(deps): configure Renovate for Go, pub, and weekly Nix

EOF
)"
```

Expected: commit succeeds on `renovate/configure`.

---

### Task 2: Push and verify the onboarding PR

**Files:**
- None (git remote + `gh` only)

**Interfaces:**
- Consumes: Task 1 commit on `renovate/configure`
- Produces: updated PR #4 on GitHub with the new `renovate.json`

- [ ] **Step 1: Push the branch**

```bash
git push -u origin HEAD
```

Expected: `renovate/configure` updates on `origin` (may be ahead by design-doc + config commits).

- [ ] **Step 2: Confirm PR #4 still open and lists the config change**

```bash
gh pr view 4 --json title,state,url,files
```

Expected: `state` is `OPEN`; `files` includes `renovate.json` (and may include the design spec if that commit was pushed).

- [ ] **Step 3: Spot-check PR file content**

```bash
gh pr diff 4 -- renovate.json
```

Expected: diff shows Nix enablement, `group:allNonMajor`, semantic commit presets, `labels`, `lockFileMaintenance`, and the two weekly Nix `packageRules`.

- [ ] **Step 4: Manual acceptance checklist (no further code)**

Confirm against the spec:

1. Managers: gomod + pub (default) + nix enabled.
2. Non-majors grouped; majors separate (`group:allNonMajor`).
3. Go/pub not given a weekly schedule (immediate).
4. Nix manager + `flake.lock` lockFileMaintenance scheduled `before 6am on monday`.
5. `automerge` false.
6. Labels `dependencies`; commits `chore` + scope `deps`.

After merge of PR #4, Renovate’s next repository job will rewrite the Dependency Dashboard / open update PRs as needed. No app tests are required for this config-only change.

---

## Spec coverage (self-review)

| Spec requirement | Task |
| --- | --- |
| Enable gomod + pub (already detected) | Task 1 (extends recommended) |
| Enable nix / flake.lock | Task 1 (`nix.enabled` + lockFileMaintenance rule) |
| Balanced grouping (non-major grouped, majors separate) | Task 1 (`group:allNonMajor`) |
| Go/pub immediate | Task 1 (no schedule on those managers) |
| Nix weekly `before 6am on monday` | Task 1 (`packageRules`) |
| `chore(deps)` commits | Task 1 (semantic commit presets) |
| `dependencies` label | Task 1 (`labels`) |
| Automerge off | Task 1 (`automerge: false`) |
| Lockfile maintenance for Go/pub | Task 1 (`lockFileMaintenance.enabled`) |
| Deliverable on PR #4 | Task 2 (push + `gh pr view`) |
| No skills-lock / Actions / Docker | Global Constraints (omitted from config) |
