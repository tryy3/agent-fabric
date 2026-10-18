# Built-in skills

Each folder under `builtin/` is one skill: a `SKILL.md` (frontmatter plus
instructions) and optional `references/`, `scripts/` and `assets/` files.
They are embedded into the control plane binary and shown read-only; to
change one, copy it into a project's `.agent-fabric/skills/` folder.

| Skill | What it does |
| --- | --- |
| `deep-research` | Researches a question on the web over several rounds and writes a cited Markdown report under `research/<slug>/` in the project files. Needs `web_search` and `fetch_page`. |

See `docs/skills-implementation-plan.md` for the format and the layering rules.
