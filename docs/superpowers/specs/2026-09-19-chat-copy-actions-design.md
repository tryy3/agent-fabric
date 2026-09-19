# Chat copy actions per container

**Date:** 2026-09-19  
**Status:** approved for planning  
**Parent:** GitHub issue #25 (copy actions checklist), PR #29  
**Builds on:** [2026-09-15-chat-document-layout-design.md](./2026-09-15-chat-document-layout-design.md), [2026-09-19-markdown-selection-copy-design.md](./2026-09-19-markdown-selection-copy-design.md)

## Problem

Transcript containers (user prompt, thinking, tool call, assistant message) are selectable, but there is no one-click way to copy a whole container. Users must drag-select, which is awkward for collapsed activity rows and for structured tool payloads.

Hermes-style UIs put a subtle always-available copy control on each surface and, for tools, copy a stable structured dump rather than whichever tab is open.

## Goals

- Add an always-visible **Copy** control on:
  - User prompt (footer under the bubble)
  - Thinking activity (header, before chevron)
  - Tool activity (header, before chevron)
  - Assistant message (footer row next to `model · provider · tok/s`)
- Tool copy always puts a structured full dump on the clipboard, independent of the active tab.
- Retarget tool tabs from **Input / Output** to **Full / Output**, with **Full** as the default open tab.
- Keep expand/collapse, selection, and Stats behavior intact.
- Leave room for later footer actions (edit / fork) without redesigning placement.

## Non-goals

- Edit, fork, retry, or other non-copy actions (placeholders out of UI).
- Hover-only / focus-only reveal of actions.
- Separate Input vs Output copy buttons.
- Transforming whole-message copy into “visual plain text” (selection copy already handles multi-block markdown; whole-container copy uses source `bubble.text`).
- Copy for Stats fields.
- ACP / control-plane / persistence changes.
- Agent avatar / “Trace: N tools” chrome.

## Decision

| Topic | Choice |
|-------|--------|
| Affordance | Always-visible icon button (Approach A) |
| User / assistant placement | Footer action row under the content (same family as caption + Stats) |
| Thinking / tool placement | Header row, before expand chevron |
| Tool clipboard | Structured dump: `tool` / `args` / `output` — always, any tab |
| Tool tabs | **Full** / **Output** (replaces Input / Output) |
| Full tab body | Labeled stacked sections (Tool, Args, Output) |
| Default tool tab | **Full** |
| Assistant / user clipboard | Source `bubble.text` |
| Thinking clipboard | Full thought text even when collapsed |
| Empty content | Copy whatever is present (including mid-stream); no-op only if truly empty |
| Feedback | Short snackbar after successful copy (match link-copy pattern) |

## Layout

```text
[ user bubble ───────────── ]
  [ Copy ]                         ← trailing under bubble

▸ Thinking  short desc     [Copy] ›
▸ tool_name  status        [Copy] ›

Agent prose…
model · provider · tok/s   [Copy]
[ Stats ]
```

## Tool tabs & copy format

### Tabs

When `toolIO` shows both panes (`both` / future `full+output`):

| Tab | Body |
| --- | --- |
| **Full** | Labeled stacked sections: Tool title, Args (formatted input), Output (formatted output). Monospace for args/output values. |
| **Output** | Formatted output only (current monospace body). |

- Default selected tab: **Full** (including when output already exists).
- Single-mode view modes that force one pane map: `full` → Full body only; `output` → Output body only; no tab chrome.
- Rename `ToolIOMode.input` → `ToolIOMode.full` (or equivalent). Built-in Pretty/Detailed modes keep `both`.

### Clipboard (any tab)

```text
tool: <title>
args: <formatted input>
output:
<formatted output>
```

- Title = tool title (fallback `Tool call`).
- Input/output use the same formatting helper as the UI (`_formatToolValue` today).
- Missing values still emit the label with `—` as the body (same sentinel the tool UI already uses for empty panes). Full-tab empty sections use the same sentinel.
- Copy does **not** depend on which tab is selected.

Users who need a subset expand the tool and drag-select as today.

## Components

| Piece | Job |
| --- | --- |
| `CopyAction` (or similar under `client/lib/chat/`) | Compact icon button + tooltip/semantics “Copy”; writes clipboard; shows snackbar; stops header tap from toggling expand |
| `formatToolCopyText` (+ optional Full-section model) | Pure helpers shared by clipboard and Full tab so shape stays testable |
| `chat_screen.dart` | User bubble: wrap with footer `CopyAction` under the container |
| `agent_bubble.dart` | Thinking/tool headers + message caption row |
| `view_modes.dart` | `ToolIOMode` rename `input` → `full`; update defaults/tests |

## Interaction details

- Copy control sits inside the activity header but must **not** toggle expand/collapse (absorb tap).
- Accessibility: tooltip and/or `Semantics` label “Copy”; do not rely on icon alone.
- Snackbar copy confirms success; omit noisy toasts on empty no-op.
- Stats control stays as today; Copy sits on the caption row, Stats remains the separate chip below when usage exists.

## Testing

- Unit: `formatToolCopyText` produces the structured shape for typical input/output maps and empty cases.
- Widget: Copy affordances present on user, thinking, tool, and assistant surfaces.
- Widget: Tool tabs labeled Full / Output; default tab Full when both shown.
- Widget: Tapping Copy on a collapsed thinking/tool row does not expand it.
- Existing selection / markdown / Stats tests remain green.

## Acceptance

- Each of the four surfaces has a working Copy control as placed above.
- Tool copy always yields the structured dump; Full tab shows labeled stacked sections; default tab is Full.
- No hover gate; no edit/fork; selection inside bodies still works.
