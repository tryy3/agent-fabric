---
name: gh-create-issue
description: >-
  Create a GitHub issue for the current repository with correct template format,
  type/area/impact labels, and actionable content. Use when the user asks to create
  an issue, file a bug, request a feature, or capture work as a GitHub issue.
---

# GitHub Create Issue

Create issues that follow repository templates and label policy.

## Workflow

### Step 1: Determine template type

Analyze the user's request:

| User intent | Template |
|-------------|----------|
| Bug, error, crash, something not working | Bug Report (`0_bug_report.yml`) |
| New feature, enhancement | Feature Request (`1_feature_request.yml`) |
| Question, help, discussion | Questions & Discussion (`2_question.yml`) |
| Does not fit above | Other (`3_others.yml`) |

**If unclear**, ask which template to use. Do not default to "Others" on your own.

### Step 2: Read the selected template

Read `.github/ISSUE_TEMPLATE/<file>.yml` for:

- Title prefix (`title` field)
- Template labels (`labels` field)
- Required sections and fields

### Step 3: Collect information

Infer as much as possible from the user's request. Only ask when too vague to write a meaningful issue (e.g. no reproducible detail, unclear desired outcome). Do not ask about optional fields.

### Step 4: Determine labels

Apply **all** applicable labels at creation time.

#### From template

Always include template `labels` (e.g. `type/fix`, `type/feature`, `discussion`).

#### Area labels (required for bugs and features)

Infer component from the user's description. Apply one or more `area/*` labels:

| Component / scope | Label |
|-------------------|-------|
| Control plane, Go server, ACP `/acp`, catalog `/v1`, Postgres, providers | `area/controlplane` |
| Flutter client, cockpit UI, chat, settings screens | `area/client` |
| Sandbox, Docker/Podman runtime, local FS workspace | `area/sandbox` |
| Deploy compose, GHCR images, nginx, Tailscale Serve | `area/deploy` |
| Documentation, README, AGENTS.md, DESIGN.md | `area/docs` |
| CI, GitHub Actions, Nix flake, Renovate | `area/ci` |

Use multiple `area/*` labels only when the issue truly spans areas.

#### Impact labels (when applicable)

| Situation | Label |
|-----------|-------|
| Breaking change, migration required | `impact/breaking` |
| Security vulnerability or hardening | `impact/security` |
| Deployment, ops, production concern | `impact/ops` |

#### Type label

Keep exactly one `type/*` label. Template labels usually provide this (`type/fix`, `type/feature`). For "Others" template with no type label, infer from content.

#### Questions / discussions

Apply `discussion` and/or `help wanted` from the question template. Do **not** use bug/feature type labels for pure questions.

### Step 5: Build issue content

Write the body to a fixed path — do **not** use `mktemp` + heredoc across separate terminal calls:

```bash
# write content to /tmp/gh-issue-body.md using the Write tool or a single shell command
```

- Use the exact title prefix from the template.
- Fill all required template sections in order.
- "Good-enough" detail is fine — capture the idea clearly.
- Include **Component** and **Impact** sections in the body when using bug/feature templates (mirrors the GitHub form fields).

**Do NOT preview or ask for confirmation** unless the user explicitly requests it or the request is too vague.

### Step 6: Create issue

Build one `--label` flag per label. Example:

```bash
gh issue create \
  --title "[Bug]: Session drops when catalog provider is missing" \
  --body-file /tmp/gh-issue-body.md \
  --label "type/fix" \
  --label "area/controlplane" \
  && rm -f /tmp/gh-issue-body.md
```

Use `--web` only when complex formatting needs the browser:

```bash
gh issue create --web
```

### Step 7: Report

Report the created issue URL and applied labels.

## Title guidance

- Specific and searchable — include symptom and affected component.
- Avoid vague titles: "Bug", "Issue", "Help", "Not working".

## Policy references

- `AGENTS.md`, `README.md`, `docs/architecture.md`
- Label taxonomy: exactly one `type/*`, one or more `area/*`, optional `impact/*`
