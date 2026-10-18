# Deep research mode and a skills foundation for agent-fabric

Status: design only, no code. Date: 2026-10-16.

## 1. How Odysseus does it

Odysseus is the self-hosted AI workspace associated with PewDiePie (repo `odysseus-dev/odysseus`; sources disagree on the license, so check before reusing any code).

**Origin of the method.** `ACKNOWLEDGMENTS.md` credits Alibaba-NLP's Tongyi DeepResearch (Apache-2.0) for the "multi-step deep-research agent pipeline". It lists `services/research/`, `src/research_handler.py`, `routes/research_routes.py` and `services/search/` as derived. The `DeepResearcher` docstring calls the loop "IterResearch-style". IterResearch comes from the WebResearcher paper (arXiv 2509.13309). Its idea is to treat research as a Markov decision process: each round sees only a bounded workspace plus one **evolving report** that is rewritten every round, instead of an ever-growing transcript.

**Pipeline** (read from `src/deep_research.py` and `research_handler.py`; the control flow is fixed code, the LLM only makes content decisions and nothing here uses model tool-calling):

1. Plan: one LLM call returns JSON with sub-questions, key topics and success criteria. A category (product, comparison, howto, factcheck) is auto-detected and swaps in a format override.
2. Each round, up to 8 rounds or 300 s:
    - Generate queries: 4 in round 1, 3 after, deduplicated against earlier queries.
    - Run the searches in parallel (up to 10 results each, with a provider fallback chain).
    - Fetch pages and run a per-page extraction LLM call under a concurrency semaphore. Page text is wrapped as untrusted context, and low-quality extractions are dropped.
    - Synthesize: merge the newest 10 findings plus the current report into a new report, with inline URL citations. If this fails, the previous report is kept.
    - Stop check: after a minimum of 2 rounds the LLM answers a YES/NO stop prompt. Two empty rounds in a row count as "search is down" and end the run.
3. A final report pass writes a long, sectioned article with an executive summary and conclusion. Output under 400 words triggers one expand call.
4. Output is Markdown with a stats line, `### Sources`, `### Analyzed URLs` (an audit list) and a `<details>` block of raw findings. It is saved as JSON per session.
5. Runs as a background task with status, cancel and progress callbacks, and falls back to a legacy orchestrator or plain search on error.

**The HTML report (confirmed, added 2026-10-18).** You remembered right: the research itself ends as Markdown, and the look is applied afterwards by a separate renderer. `GET /api/research/report/{id}` calls `get_report_html`, which calls `generate_visual_report` in `src/visual_report.py`. That function makes no LLM call. It takes the question, the Markdown report, the sources, stats and the detected category, and returns one self-contained HTML page:

- Markdown is rendered with python-markdown (extra, toc, tables, code highlighting) and then sanitized with `nh3`.
- **Hero image:** the first usable `og_image` collected from the sources the research fetched. It is not generated. Filters: https only, not svg/ico/gif, not a logo/icon/favicon/sprite/badge URL, not on a small blocklist, not duplicated, not user-hidden. Later images are placed after every second section heading, and leftovers are a pool for a "reroll" button. The Library list uses the same pick as its thumbnail.
- **Backgrounds:** pure CSS. An animated "aurora" of three blurred radial gradients plus a film-grain overlay, light and dark themes, reduced-motion respected. The detected category (product, comparison, how-to, fact-check/landscape) adds its own palette, font stack and background (blueprint grid, dot grid, vignette, drifting bands) and some category-specific layout.
- **Layout and extras:** sticky table of contents with scroll tracking, drop cap, collapsible source list, hide/restore image, PDF export (browser print) and HTML download, and a "Discuss" button that starts a chat seeded with the report only (no sources, to save tokens and avoid invented citations).

So the cool part is deterministic templating over the Markdown plus images scraped from the sources, which is cheap to copy.

## 2. What agent-fabric has today

- The turn loop is capped at **8 tool rounds per Prompt** (`agent.go:703`). A research run needs far more work than that.
- Tool I/O is not replayed to the next Prompt's LLM; only visible text and reasoning are.
- Plane-owned `web_search` and `fetch_page` exist with SSRF-safe fetch, Gate scoring and hop capture (`docs/decisions.md`, `sandbox/tools/web`).
- Execution is server-side. There is a project workspace and an execution environment, and clients do not own the agent loop or the system prompt.
- There are no slash commands, skills or sub-agents yet. The repo's `skills-lock.json` is for Claude Code dev skills and is unrelated.

## 3. How other harnesses do built-ins

- **Claude Code and similar:** a skill is a folder with a `SKILL.md` (name, description, instructions, optional scripts). The user invokes `/name args`, or the model loads it when the description matches. The skill text is injected into the turn and the normal tool loop runs.
- **Slash commands:** a client-side parser sends a name plus args. ACP has a first-class slot for this: the Agent advertises `available_commands` through `session/update`, and the client shows them in a picker. The prompt arrives as ordinary text starting with `/name`.
- **Dedicated pipelines** (Odysseus, ChatGPT/Gemini deep research): a code-driven orchestrator with its own budget, outside the normal chat loop.

## 4. Options

**A. Odysseus-style fixed pipeline in Go.** `/deep-research` routes to a coded orchestrator (plan, rounds, synthesize, report). Predictable and cheap to bound, with its own round budget. But it is a one-off: nothing is reusable for skills.

**B. Pure skill, prompt-only.** `/deep-research` loads a SKILL.md prompt and the normal tool loop does the work. Smallest build and the best skills foundation, but the 8-round cap and the lack of tool I/O replay make it produce shallow results. Quality depends on the model.

**C. Skills layer plus a plane-owned research runner (recommended).** Build a small skills system whose first entry, `deep-research`, has a `runner: research` that hands off to a coded IterResearch-style loop. A prompt-only skill would just be a skill without a runner. This gives A's depth and B's extensibility.

## 5. Recommendation: Option C (sections 5.1 and 5.2 are superseded by section 8)

### 5.1 Skills foundation

- A **skill** is catalog data: `name`, `description`, `argumentHint`, `instructions` (Markdown), optional `runner`, `enabled`. Built-ins ship embedded in the binary; user-defined ones live in Postgres via catalog `/v1/skills`, plane-wide first, then per project or assistant later.
- Skills are listed to the client as ACP `available_commands`, pinned at `session/new` like other settings. The client only renders the picker; it holds no skill logic (matches the client boundary rule).
- Invocation: a prompt beginning with `/name args` is resolved by the plane. Prompt-only skills inject their instructions into the turn. Runner skills start the runner.
- Needs a `docs/terminology.md` (and `.sv.md`) entry for "skill" in the same change.

### 5.2 The research runner

- Plan, then rounds of query generation, search, fetch, per-page extraction, report rewrite, stop check, then a final report. This mirrors the IterResearch idea: carry the evolving report, not the transcript.
- Reuses the plane's pinned `web_search` and `fetch_page` bindings, Gate, and hop capture. Each LLM call and tool call is captured, as the capture rule requires.
- Uses its own budget (rounds, wall-clock, tokens) instead of the 8-round turn cap. Progress streams as ACP plan updates and tool-call updates; cancel maps to ACP cancel.
- Page text is treated as untrusted (prompt-injection defense), and the final report cites only fetched URLs.
- Needs no network from the sandbox, because it goes through plane tools.

### 5.3 The HTML report

Follow the Odysseus split: research produces Markdown, and a separate deterministic renderer turns it into HTML, with no LLM and no model-written HTML.

- The fetch step records each page's `og:image` (and title/domain) alongside the extracted text. The renderer picks a hero image with the same filters as Odysseus (https, no svg/ico/gif, no logos/icons, de-duplicated), places a few more between sections, and keeps a spare pool for a reroll.
- A fixed template adds the CSS-only look: gradient/aurora background with grain, light and dark themes, per-category palette and background, sticky table of contents, numbered sources, an audit list of analyzed URLs, reduced-motion support. Use the repo's `DESIGN.md` tokens for the base theme and let the category only vary the accent and background.
- Sanitize rendered Markdown (allowlist, no scripts from the model) and show the page in a sandboxed iframe. Images are remote hotlinks, so either proxy them through the plane's SSRF-safe fetch or accept hotlinking; Odysseus hotlinks.
- Render at view time from the saved Markdown plus sources JSON (as Odysseus does), so restyling the template restyles old reports. Optionally also save a downloadable `reports/<slug>.html`.
- Where no project workspace exists, keep the report with the thread.

### 5.4 Defaults I picked (all reversible)

1. The runner uses the assistant's own model and inference settings; no separate research model.
2. Defaults: 6 rounds, 5 minutes, then a final report; configurable per skill.
3. Plane-rendered HTML from Markdown at view time, hero image taken from fetched sources' og:image, not generated.
4. Built-in skills are read-only; user skills come later.
5. Slash command goes through ACP `available_commands`, not a custom client-side protocol.
6. The command works as the first message of a fresh thread, as you described.

## 6. Phasing

1. Skills data model, `available_commands`, prompt-only skill invocation, client picker. One tiny built-in skill to prove it.
2. Research runner with Markdown output and progress streaming.
3. HTML renderer, workspace save, and preview in the client.
4. User-defined skills in Settings; optional per-skill tool allowlists.

## 7. Open questions

- Should research be allowed without a project workspace, with the report as a chat-only attachment?
- Is one model for the whole run fine, or do you want a cheaper model for page extraction and a stronger one for synthesis?
- Should `/deep-research` take flags (depth, domain allowlist), or stay `/deep-research <question>`?
- Do you want the research to ask a clarifying question first (via `ask_user`) when the question is vague?
- Skills scope: plane-wide only at first, or per project and per assistant from day one?
- Is a SKILL.md-compatible file format (so Claude Code style skills import directly) worth the constraint?
- License check on Odysseus before borrowing any code; I recommend reimplementing from the paper and the described flow instead.

## Sources

- [Odysseus repo](https://github.com/odysseus-dev/odysseus) and its `ACKNOWLEDGMENTS.md`, `src/visual_report.py`, `routes/research/research_routes.py`, `src/deep_research.py`, `src/research_handler.py`, `services/research/service.py`
- [WebResearcher / IterResearch, arXiv 2509.13309](https://arxiv.org/pdf/2509.13309v2)
- [Alibaba-NLP/DeepResearch](https://github.com/Alibaba-nlp/DeepResearch)
- Reviews: [Android Authority hands-on](https://www.androidauthority.com/pewdiepie-odysseus-ai-hands-on-3682909/), [MindStudio](https://www.mindstudio.ai/blog/what-is-odysseus-pewdiepie-open-source-ai-workspace)

## 8. Revised recommendation (2026-10-18): skills as files, no research "runner" in core

Feedback: keep the core small, make built-ins inspectable and replaceable, follow the Agent Skills / Pi / Nanocoder direction. That removes the coded research runner from Option C. Deep research becomes a **built-in skill** that the normal agent loop executes. Note: I could not read the Nanocoder or Pi docs this round (the lookups were declined), so the comparisons below are from general knowledge and should be checked before we copy any detail.

### 8.1 What a skill is

A folder:

```
deep-research/
  SKILL.md          frontmatter (name, description, argument hint, optional settings) + instructions
  references/       extra Markdown the model reads only when SKILL.md tells it to
  scripts/          optional helpers the model runs in the sandbox (e.g. render_report)
  assets/           templates (the report HTML/CSS template)
```

- Progressive disclosure: only each skill's name and description go into the system prompt. The body is loaded when the skill is invoked (`/deep-research ...`) or when the model decides it matches. References and scripts are read or run on demand.
- A skill can also be a plain one-file **command** (Nanocoder-style Markdown prompt with an argument placeholder). Same loader, same toggles; a command is just a skill without extra files.
- Stay compatible with the common SKILL.md format so existing skills import unchanged.

### 8.2 Where skills live, and who wins

1. Built-in: embedded in the binary from a normal directory in the repo (`controlplane/.../skills/builtin/deep-research/`), so it is real files, reviewable in git.
2. Plane-wide user skills: stored on the plane (data dir or Postgres, to decide), editable in Settings.
3. Project skills: `.agent-fabric/skills/` (and read `.agents/skills/` too) inside the project workspace, so they can live in the repo and be edited by the agent or the user.
4. Per-assistant enable/disable and overrides.
   Precedence: assistant > project > plane > built-in; a same-named skill replaces the one below it. "Customize built-in" copies the files into the project or plane layer.

### 8.3 Transparency and control

- Settings lists every skill with source (built-in / plane / project), full text viewable, and an on/off toggle at plane, project and assistant level. Turning off a built-in removes it from the prompt and the slash menu entirely.
- The effective-instructions preview (already deferred in decisions) must show the skill list that was pinned. Skills are pinned at `session/new` like other settings.
- Every skill load, reference read and script run appears as a normal tool call in the transcript and in the hop capture. Scripts run only through the existing sandbox and Gate, never in the plane process.

### 8.4 What the core has to provide (the whole list)

1. Skill discovery, layering, toggles and pinning (catalog `/v1/skills`, read-only for built-ins).
2. A way to load a skill: either the model calls a small `load_skill` tool, or it reads `SKILL.md` through the normal file tools (this reuses the sandbox and Gate; prefer it if the skill files are mounted read-only into the sandbox).
3. ACP `available_commands` so `/deep-research` shows up in the client, and `/name args` resolution on the plane.
4. A per-skill (or per-assistant) **tool-round budget**, because the hard cap is 8 per Prompt today. This is the one real core change research needs.
5. Rendering an `.html` report file in the client in a sandboxed iframe (scripts off).
   No research logic, no model-specific prompts and no HTML templating live in Go.

### 8.5 How deep-research works as a skill

- SKILL.md describes the IterResearch idea in plain instructions: make a plan, then in each round run searches (`web_search`), read selected pages (`fetch_page`), and **rewrite a running notes/report file in the workspace** (`research/<slug>/report.md`) instead of relying on chat history. This matters because tool I/O is not replayed to later Prompts; the file is the memory. Stop when the success criteria are met or the budget runs out, then write the final report.
- Sources go into `research/<slug>/sources.json` (url, title, image) as the model goes.
- `scripts/render_report` (in the sandbox, no network needed) turns `report.md` + `sources.json` + `assets/template.html` into `report.html` with the hero image, gradients, table of contents and category styling. The template is a plain file the user can edit or replace.
- Small core tweak this needs: `fetch_page` should return the page's `og:image` in its metadata (it does not today), so the skill can record a hero image.
- Cost: no separate model or hidden loop; it uses the assistant's model and budgets. Depth is lower and less predictable than a coded loop, and quality depends on the model following the instructions. If that proves too weak later, a coded loop can be added as a plain tool the skill calls, without changing the skill system.

### 8.6 Defaults I picked (reversible)

1. Skills are folders with SKILL.md plus references/scripts/assets; single-file commands are the degenerate case.
2. Built-ins ship as files in the repo and are shown read-only; customizing copies them.
3. Skill instructions are loaded on demand, not injected in full.
4. Scripts run only in the sandbox through Gate.
5. No research runner in core; add one later only if skill-driven research is too weak.
6. Report HTML comes from a script and template inside the skill, not from Go.

### 8.7 New open questions

- Does a skill need a project workspace to run (it writes files)? Without a project, do we use a scratch workspace per thread?
- Plane-wide user skills: files in the data dir or rows in Postgres? Files are more transparent; Postgres is easier to edit in Settings.
- Should the model load skills itself (`load_skill` or file read) or only the user via `/`? Recommendation: both, with a per-skill "user-invocable only" flag.
- Round budget: per-skill frontmatter, per-assistant setting, or both?
- Is the sandbox always available? Skills with scripts need it; a skill could declare `requires: sandbox` and be hidden otherwise.

## 9. Decisions from the 2026-10-18 follow-up

1. **Workspace:** both. A skill argument chooses where output goes (e.g. `/deep-research --save project|scratch <question>`). Default: project workspace when the thread has one, otherwise a per-thread scratch workspace. The scratch workspace is deleted with the thread and never touches project files.
2. **Plane-wide skills:** start easy. Built-in skills live in their own directory/module of the mono-repo (e.g. `skills/`) so they can be split out later. Later option: download them into a persistent folder instead of embedding in the Go binary. The loader reads from folders either way, so the source can change without changing skills.
3. **Skill loading, v1:** `/name args` from the user and a `load_skill` tool for the model. Anything more (auto-matching, per-skill flags) waits until we have used it.
4. **Round budget** (explained): one "tool round" is the model asking for tools and getting results back. A Prompt currently stops after 8 rounds. Deep research needs dozens, so the limit has to be raisable for a skill run (per-skill frontmatter value, capped by an assistant/plane maximum). That is the only core change the loop itself needs.
5. **Where scripts run:** not fixed to the project sandbox. A skill states what it needs (`none`, `sandbox`), and the plane picks a place: the project's execution environment, a throwaway scratch sandbox, or later a lighter dedicated one. v1: use whatever execution environment the thread has; if none and the skill needs one, the skill is hidden or says so. The report renderer is deterministic and needs no network, so it is the first candidate for a lightweight runner.

## 10. Budget model (2026-10-18)

Two different numbers, kept apart:

- **Tool-round limit (core, hard):** how many model-and-tools rounds one skill run may use, plus a wall-clock limit. The core enforces it; a skill cannot exceed it.
- **Research rounds (skill, soft):** how many search-read-rewrite cycles deep-research plans for. The skill instructions follow the number given in the argument and stop early when the success criteria are met. Each research round costs several tool rounds, so the skill states a rough conversion.

Precedence for the hard limit:

1. **Plane maximum** (Settings): absolute ceiling, nothing exceeds it.
2. **Assistant setting**: may raise or lower the limit for that assistant, never above the plane maximum.
3. **Skill frontmatter**: the skill's default and its own ceiling (e.g. `maxToolRounds: 60`), applied inside the assistant limit.
4. **Run argument**: the user picks within what is left, e.g. `/deep-research --rounds 6 --time 10m <question>`, plus a preset `--depth quick|normal|deep`. Values above the effective limit are clamped and the user is told.

Effective limit = min(plane max, assistant limit, skill ceiling, run argument). The thread shows the effective limit and how much is used, and a run that hits the limit writes the best report so far instead of failing.

Odysseus (from the code I read): 8 research rounds and 300 seconds by default, with a minimum of 2 rounds before an LLM stop check. I did not confirm the exact user-facing setting. Our defaults stay at 6 rounds and 5 minutes for `normal`.
