# Skills and `/deep-research`: implementation plan

Status: plan, no code yet. Date: 2026-10-18. Design and decisions: [deep-research.md](deep-research.md) (sections 8 to 10 are the current design).

## Scope

In:

- A skill system: folders with `SKILL.md`, optional extra files, layered sources, on/off toggles, `/name args` from the user and `load_skill` for the model.
- A tool-round budget: plane maximum, assistant override, skill ceiling, run flags.
- A built-in `deep-research` skill that writes a cited Markdown report.

Out for now (decided): the HTML report, images, user-authored plane skills in Settings, model auto-matching of skills, and new lightweight runners for scripts.

## What exists today (checked in the code)

- Every thread belongs to a Project, and every Project resolves to an execution environment: `local` (`{dataDir}/projects/{id}/workspace`) or `docker` (volume `agent-fabric.proj.{id}`). See `docs/architecture.md` "Tools and sandbox backends".
- The turn loop hard-codes the round cap: `maxRounds = 8` when tools exist (`controlplane/internal/agent/agent.go:701-703`).
- Tools are registered through `sandbox.Registry` (`internal/sandbox/tools/defaults.go`), with origins `environment`, `control_plane` and `mcp`. `web_search` and `fetch_page` are pinned per session (`internal/sandbox/tools/web`).
- Effective instructions are composed and pinned at `session/new` (`agent.go`, `ComposeEffectiveInstructions`).
- The ACP SDK (`coder/acp-go-sdk` v0.13.5) already has `AvailableCommand` and `AvailableCommandInput`, so the plane only has to send `available_commands_update`. The Flutter client does not handle it yet.
- The client already renders Markdown (`flutter_markdown_plus`) and has a project file explorer with a preview pane, so a `report.md` written to project files can be viewed without new UI.
- There is no scratch workspace and no skill code yet.

## Phases

Each phase ends in its own PR, with green CI, and is usable without the next phase.

### Phase 1: skill format and loader (Go, no UI)

- New `skills/` folder at the repo root, as its own small Go module (`skills/embed.go` with `//go:embed`). `go:embed` cannot reach outside `controlplane/`, and a separate module keeps the skills easy to move out later. Wire it into `controlplane` with a `replace` (or `go.work`).
- `internal/skills` package:
    - parses `SKILL.md` frontmatter: `name`, `description`, `argumentHint`, `flags`, `maxToolRounds`, `requires` (`none` | `sandbox`). Unknown keys are ignored.
    - reads the body and lists the extra files in the folder.
    - layers sources with precedence built-in < plane < project < assistant, where the same name replaces the one below. v1 sources are the built-ins (embedded) and project files under `.agent-fabric/skills/` and `.agents/skills/`.
    - applies toggles from plane settings, project settings and assistant `settings.skills` (`{"disabled": ["name"]}`).
- Pin the resolved skill list (names, descriptions, source, content hash) at `session/new`, next to the other pinned settings.
- Catalog read API: `GET /v1/skills?projectId=&assistantId=` (effective list with source and enabled state) and `GET /v1/skills/{name}` (full text). Built-ins are read-only.
- Add the term "skill" to `docs/terminology.md` and `docs/terminology.sv.md`, and a decision entry in `docs/decisions.md`.
- Tests: frontmatter parsing, layering and override, toggles, pinning snapshot.

### Phase 2: invoking skills (Go)

- `load_skill` tool (origin `control_plane`, read-only). `load_skill(name)` returns `SKILL.md`, and `load_skill(name, path)` returns one extra file from that skill. Add it to the Gate as a read-only tier and update `controlplane/bench/gate/cases/`, then run `cmd/gatebench`, as the repo rules require.
- System prompt: append a short "Available skills" list (name and description only) to the pinned effective instructions, only when at least one skill is enabled.
- `/name args` in `session/prompt`: if the first text block starts with `/<enabled skill name>`, the plane parses the declared flags, injects the skill body as a user-side context part for that Prompt, and passes the rest as the question. An unknown `/name` is sent unchanged as normal text.
- Flag parsing: only flags declared in frontmatter (`--depth quick|normal|deep`, `--rounds N`, `--time 10m`, `--save project|scratch`). A bad value gets one plain error message back, with no model call.
- Send ACP `available_commands_update` after `session/new` and when the pin changes.
- Persist the invocation (skill name, source, hash, parsed flags) on the Prompt so the transcript and the chat inspector show it.
- Tests with the scripted provider: slash invocation, unknown command, bad flag, `load_skill` result, `available_commands` sent.

### Phase 3: tool-round budget (Go + Settings UI)

- Replace the hard-coded 8 with an effective limit: min(plane maximum, assistant limit, skill `maxToolRounds`, `--rounds`/`--depth` mapping). Same model for wall-clock (`--time`, plane and assistant maxima).
- Settings: plane `limits.defaultToolRounds` (8, today's behavior for normal Prompts), `limits.maxToolRounds` (ceiling, default 100) and `limits.maxTurnSeconds` (default 15 minutes). Assistant `settings.limits` has the same fields, merged by catalog PATCH and pinned at `session/new`. A normal Prompt uses the default; a skill run may go up to its own ceiling within the maxima.
- When the limit is reached, the loop gets one final round without tools, so the model writes its best answer instead of erroring. Today the loop stops with an error after the cap.
- Stream usage ("round 14 of 40") through the existing ACP plan or tool updates.
- Settings UI: two number fields on plane settings and the assistant editor (follow `DESIGN.md`).
- Tests: clamping order, final no-tool round, wall-clock stop.

### Phase 4: `/deep-research` skill (skill content plus small Go work)

- `skills/deep-research/SKILL.md`: the IterResearch-style loop in plain instructions:
    - plan sub-questions and success criteria;
    - for each round, search with `web_search`, read the best pages with `fetch_page`, and rewrite `research/<slug>/report.md` with inline citations;
    - keep `research/<slug>/sources.md` with every URL read;
    - stop when the criteria are met or the round count is reached, then write the final report (summary, sections, conclusion, sources).
- `references/report-format.md` (section layout, citation style) and `references/categories.md` (product, comparison, how-to, fact-check), loaded on demand.
- Flags: `--depth quick|normal|deep` (about 3, 6 or 10 research rounds), `--rounds N`, `--time`, `--save project|scratch`. The default is `normal`, saved to project files. `maxToolRounds: 80` as the skill ceiling, still capped by the plane and assistant.
- `--save project` writes under `research/` in project files. `--save scratch` needs phase 5, and until then it answers that scratch is not available yet.
- The final chat message is a short summary with the report path, and the file opens in the existing Markdown preview.
- Tests: a scripted-provider run that checks the flags reach the model, the budget is applied and the files are written.
- Manual check: one real run on the live stack (`scripts/cloud/up.sh`) with SearXNG or Linkup.

### Phase 5: scratch workspace (Go, small UI)

- A per-thread directory that lives outside project files and is deleted with the thread. Local: `{dataDir}/threads/{id}/scratch`. Docker: a second mount at `/scratch` from volume `agent-fabric.thread.{id}`.
- File tools accept a `scratch/` prefix (or a `root` argument) that maps to it, and the path jail covers both roots.
- The client shows a "Scratch" section in the file explorer for the open thread.
- Needs a short spike first: whether a second root fits the file tools and the docker volume lifecycle cleanly. If it does not, the fallback is a hidden `.agent-fabric/scratch/<threadId>/` in project files that is removed with the thread and excluded from export and publish.

### Phase 6: client slash menu (Flutter)

- Handle `available_commands_update` in the ACP client and show a `/` picker in `chat_composer.dart` with names, descriptions and argument hints.
- A read-only Skills page in Settings: list (source, enabled), full text, and toggles at plane, project and assistant level.
- Gates: `dart format`, `flutter analyze --fatal-infos`, `flutter test`.

## Order and dependencies

Phases 1 → 2 → 3 → 4 are the critical path to typing `/deep-research <question>`. Phase 5 can run in parallel after phase 1, and phase 6 can start once phase 2 sends `available_commands`. Until phase 6 lands, typing `/deep-research ...` by hand still works.

## Risks

- **Research quality depends on the model** following the skill instructions over many rounds. Mitigation: the report file is the memory, and the phase 4 manual run decides whether a coded loop is needed later. That would be an extra tool, not a change to the skill system.
- **Context growth** within one long Prompt, because tool results accumulate across rounds. If it bites, phase 3 can add trimming of old `fetch_page` results inside a Prompt (the report file keeps the knowledge).
- **Gate prompts:** `fetch_page` on many sites may trigger asks under strict permission modes. Document this in the skill and keep the existing web tier.
- **Prompt injection from fetched pages:** the existing taint tracking (`historyTainted`) still applies. The skill tells the model to treat page text as data.

## Defaults picked (all reversible)

- Skills folder at the repo root as its own Go module, embedded in the binary for now.
- Toggles stored as a `disabled` list per layer, so new built-ins are on by default.
- Normal Prompts keep today's 8 rounds. Skill runs may go up to the plane ceiling (default 100 rounds, 15 minutes), so `/deep-research` works out of the box.
- An unknown `/name` is sent as plain text instead of an error.
- The deep-research report is Markdown in project files under `research/<slug>/`.
