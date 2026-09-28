---
name: gh-triage-issue
description: >-
  Triage a GitHub issue: apply correct type/area/impact labels, improve weak titles,
  detect duplicates, and post a structured triage comment when action is needed.
  Use when the user invokes /gh-triage-issue, asks to triage an issue, fix issue labels,
  improve an issue title, or check issue quality. Also use after manually creating an issue
  that may be missing area or impact labels.
---

# GitHub Issue Triage

Triage one or more GitHub issues so they are labeled correctly, clearly titled, and actionable.

## Policy sources

Follow these files exactly — do not invent policy:

- `AGENTS.md` / `CLAUDE.md` — product boundaries (catalog vs ACP, client boundary, sandbox split)
- `docs/architecture.md` — layers and turn lifecycle
- `docs/decisions.md` — accepted product/protocol choices
- `DESIGN.md` — UI/theme/layout (required before judging client UX issues)
- `.github/ISSUE_TEMPLATE/*.yml` — title prefixes, template labels, Component/Impact fields
- `.agents/skills/gh-create-issue/SKILL.md` — same label taxonomy used at create time

There is no `CONTRIBUTING.md` or `docs/workflow.md` in this repo; do not invent those paths.

## Prerequisites

- `gh` CLI authenticated for this repository.
- Disable pager in non-interactive runs:

```bash
export GH_PAGER=cat
export PAGER=cat
```

## Scope

- **Single issue** (default): user provides issue number, URL, or "this issue" from context.
- **Multiple issues**: user asks to triage all open issues — process each conservatively; skip issues that are already in good shape.

## Label policy

Use **existing repository labels only**. Never create new labels.

Prefer the structured taxonomy (`type/*`, `area/*`, `impact/*`) over legacy GitHub defaults (`bug`, `enhancement`, `documentation`, `question`) when both apply. If an issue only has a legacy label, replace it with the matching `type/*` label when confident.

### Type labels (exactly one when possible)

| Label | When |
|-------|------|
| `type/feature` | Feature request, enhancement |
| `type/bug` | Bug, defect, regression |
| `type/docs` | Documentation-only change request |
| `type/chore` | Tooling, housekeeping, non-user-facing maintenance |
| `type/refactor` | Internal restructuring without behavior change |
| `type/performance` | Performance improvement |
| `type/test` | Test coverage or test infrastructure |

### Area labels (one or more only when truly cross-cutting)

| Label | When |
|-------|------|
| `area/controlplane` | Go control plane, ACP `/acp`, catalog `/v1`, Postgres, providers, inference |
| `area/client` | Flutter client, cockpit UI, chat, settings screens |
| `area/sandbox` | Sandbox runtime, Docker/Podman, local FS workspace |
| `area/deploy` | Deploy compose, GHCR images, nginx, Tailscale Serve, packaging |
| `area/docs` | Documentation, README, AGENTS.md, DESIGN.md |
| `area/ci` | GitHub Actions, Nix flake, Renovate, repo automation |

### Impact labels (optional, when applicable)

| Label | When |
|-------|------|
| `impact/breaking` | Breaking change or existing setup stops working |
| `impact/security` | Security vulnerability or hardening |
| `impact/ops` | Deployment, operations, or production concern |

### Other labels

| Label | When |
|-------|------|
| `discussion`, `help wanted` | Usage help or general discussion (question template) |
| `question` | Legacy; prefer `discussion` + `help wanted` for pure Q&A |
| `duplicate` | Only when confidence is **high** — never auto-close |
| `good first issue` | Small, well-scoped starter work (rare; only when clearly true) |
| `skip-changelog` | PRs only — do not apply to issues |

### Component → area mapping (from issue templates)

When the issue body includes a **Component** answer, map it:

| Template value | Label |
|----------------|-------|
| Control plane | `area/controlplane` |
| Client (Flutter) | `area/client` |
| Sandbox | `area/sandbox` |
| Deploy / packaging | `area/deploy` |
| Documentation | `area/docs` |
| CI / Build | `area/ci` |
| Other / Unknown | Infer from title/body; skip if unclear |

### Impact → label mapping (from issue templates)

| Template value | Label |
|----------------|-------|
| Breaking change (existing setup stops working) | `impact/breaking` |
| Security vulnerability | `impact/security` |
| Operations / deployment concern | `impact/ops` |
| None of the above | No impact label |

## Workflow

### Step 1: Load the issue

```bash
gh issue view <number> --json number,title,body,labels,state,url
```

### Step 2: Assess quality

**Title** — improve only when clearly weak (e.g. "Bug", "Help", "Not working"). Good titles include symptom + affected component. Keep template prefixes (`[Bug]:`, `[Feature]:`, `[Discussion]:`, `[Other]:`) when present.

**Body** — for bugs, check for: reproduction steps, expected vs actual, version/image tag, logs, and platform.
For features, check for: problem statement, desired outcome, and architecture fit (`docs/architecture.md` / `docs/decisions.md`).

**Duplicates** — search open issues:

```bash
gh issue list --state open --limit 50 --json number,title
```

Compare title, symptom, and component overlap. Report confidence: `low`, `medium`, `high`. Only suggest `duplicate` label at **high** confidence.

**Client UX / a11y** — for `area/client` issues involving UX or accessibility, note missing details (contrast, keyboard nav, focus, labels, touch targets, screen readers, viewport) and whether the report conflicts with `DESIGN.md`.

**Architecture fit** — if a feature asks the Flutter client to own model choice, system prompt, MCP secrets, or the agent loop, flag that as out of client boundary per `AGENTS.md` (needs discussion or redesign, not a blind `type/feature` ready state).

### Step 3: Apply changes

Minimize churn — do not remove correct labels only to re-add equivalent ones.

```bash
# Update title when needed
gh issue edit <number> --title "<improved title>"

# Add labels
gh issue edit <number> --add-label "area/controlplane" --add-label "impact/security"

# Remove incorrect labels (only when clearly wrong)
gh issue edit <number> --remove-label "type/chore"
```

### Step 4: Comment only when action is needed

Post **at most one** triage comment per issue per run. Comment when at least one of:

- labels were added or removed,
- title was improved,
- quality gaps remain (clarifying questions needed),
- duplicate candidates found,
- issue conflicts with documented architecture boundaries.

If labels are correct and quality is good, **do not comment**.

Use this structure (sections in order):

```markdown
### Triage Summary
<short judgment of type, scope, readiness>

### Label Actions Taken (with rationale)
<each add/remove and why>

### Quality Gaps Found
<missing info blocking prioritization — or "None">

### Clarifying Questions
<max 5 questions, only if needed>

### Potential Duplicates
<candidate links + confidence — or "None found">

### Recommended Status Next Step
<e.g. ready-for-refinement, needs-repro, awaiting-reporter, architecture-discussion>
```

Write the comment body with the Write tool (or a single shell command) to a fixed path — do **not** use `mktemp` + heredoc across separate terminal calls:

```bash
# write content to /tmp/gh-triage-comment.md using the Write tool
gh issue comment <number> --body-file /tmp/gh-triage-comment.md \
  && rm -f /tmp/gh-triage-comment.md
```

## Operational rules

- Be conservative. Avoid noise and label churn.
- Never fabricate evidence or policy.
- Never auto-close issues.
- Report a concise summary to the user: issue URL, labels changed, title changed, whether a comment was posted.

## Related skills

- `gh-create-issue` — create new issues with correct labels from the start.
- `gh-relevance-check` — evaluate whether a stale issue or PR is still relevant.
