# Markdown selection copy (visual + newlines)

**Date:** 2026-09-19  
**Status:** approved for planning  
**Parent:** GitHub issue #25 (text selection / markdown polish)  
**Builds on:** [2026-09-18-thread-view-modes-markdown-design.md](./2026-09-18-thread-view-modes-markdown-design.md), current `MessageText` + `SelectionArea` selection fix

## Problem

Rendered markdown in chat is selectable across blocks via `SelectionArea`, but copy flattens the selection into one run-on string: Flutter concatenates each block’s plain text with no separators. Selecting a heading and the next paragraph pastes as a single line with no breaks.

Users wanted raw markdown in the clipboard (`# H1…`) for a moment, then deferred that: too hard for partial inline markup. Raw source remains available by switching to Detailed (plain) mode, or later via per-container copy actions on #25.

## Goals

- When the user copies a multi-block selection from **rendered** markdown, the clipboard keeps **visual** wording but inserts **line breaks between selected blocks**.
- Keep cross-block drag selection working (existing `SelectionArea` + `MarkdownBody(selectable: false)`).
- Stay local to the Flutter client / `MessageText`; no ACP or control-plane changes.

## Non-goals

- Copying raw / source markdown from a rendered selection (including `#`, `*`, link targets).
- Perfect reconstruction of lists, tables, or fenced code (extra tiny selectables from the markdown package may still look imperfect).
- Per-bubble copy buttons (separate #25 checklist item).
- Changing Detailed / plain `MessageText` copy behavior (already copies the source string, including its newlines).
- Thinking / tool bodies (out of this slice; tools already use `SelectableText`).

## Decision

| Topic | Choice |
|-------|--------|
| Clipboard content | Visual plain text of the selection |
| Block separators | `\n\n` between each selected selectable’s text |
| Mechanism | `SelectionContainer` transform under `SelectionArea` (SelectionTransformer-style) |
| Scope | Markdown path of `MessageText` only |
| Raw markdown | Deferred — Detailed mode or future copy affordance |

## Design

### Behavior

Given source:

```markdown
# H1 test
## H2 Test
```

Rendered as two headings. Selecting both and copying yields:

```text
H1 test

H2 Test
```

Not `# H1 test\n## H2 Test`.

Partial selection within one block still copies only the highlighted visual characters for that block (Flutter’s normal selection), then joins any other touched blocks with `\n\n`.

### Widget structure

```text
SelectionArea
  └── SelectionTransformer.separated(separator: '\n\n')   // or equivalent local helper
        └── MarkdownBody(data: text, selectable: false, …)
```

Plain / non-markdown path stays:

```text
SelectionArea
  └── Text(text)
```

### Implementation notes

- Prefer a **small local helper** under `client/lib/chat/` (e.g. `selection_transformer.dart`) adapted from the community SelectionTransformer pattern, rather than a new pub dependency. Keep only what we need (`separated` + delegate `getSelectedContent` override).
- Do not set `MarkdownBody.selectable: true` (that regresses to per-block `SelectableText` and breaks cross-block selection).
- Link tap / link dialog behavior must keep working under the same tree.

### Fallback / limitations

- If a block contributes multiple child selectables (e.g. list bullet vs item text), the transform may insert separators between those pieces. Acceptable for v1; tighten later only if chat usage shows it as noisy.
- Soft line breaks inside a single markdown paragraph remain spaces in the render (package default); copy matches what the user sees.

## Testing

- Unit-test the transform: given several plain-text fragments, joined result uses `\n\n`.
- Widget test: markdown `MessageText` still builds `SelectionArea` + non-selectable `MarkdownBody`; helper is present in the tree when practical to assert.
- Existing link / checkbox tests continue to pass.

## Acceptance

- Select heading + following paragraph in Pretty mode → paste has a blank line between the two visual strings.
- Detailed mode copy unchanged (source text).
- Cross-block selection still works; link taps still open the confirm dialog.
