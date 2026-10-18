# Built-in skills

Each folder under `builtin/` is one skill: a `SKILL.md` (frontmatter plus
instructions) and optional `references/`, `scripts/` and `assets/` files.
They are embedded into the control plane binary and shown read-only; to
change one, copy it into a project's `.agent-fabric/skills/` folder.

See `docs/skills-implementation-plan.md` for the format and the layering rules.
