# Thread view modes & markdown rendering

**Date:** 2026-09-18  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-14-chat-transparency-design.md](./2026-09-14-chat-transparency-design.md), [2026-09-15-chat-document-layout-design.md](./2026-09-15-chat-document-layout-design.md), [2026-09-14-chat-part-bubbles-design.md](./2026-09-14-chat-part-bubbles-design.md)  
**Supersedes:** Display-settings ownership of Thinking visibility (modes own it). Fulfills the deferred “markdown for agent prose” item from chat-document-layout.

## Problem

LLM replies are almost always Markdown, but the Flutter client renders assistant and user text as plain `Text`. Users also need tunable **transparency**: how much harness detail (thinking, tools, later raw requests) shows in a thread. Today Thinking visibility is a single global Display setting; tool rows are always present with only local expand state; there is no thread-scoped view mode.

Casual chat wants a clean, rendered transcript. Debugging a bad turn wants more detail without changing a global preference that affects every thread.

## Goals

- Render assistant **and** user message bodies as Markdown when the active mode requests it (`flutter_markdown_plus`).
- Introduce **configurable view modes** (hardcoded presets now; structure allows user-defined modes later).
- Ship two built-in modes: **Pretty** and **Detailed**.
- Persist the selected mode **per thread** on the control plane (`viewModeId`), with a client **app default** when the thread has no override.
- Drive transparency knobs from the active mode (markdown, thinking visibility, tool visibility, tool I/O, reserved raw-requests flag).
- Remove Thinking visibility from Display settings so modes are the single source of truth.
- Provide a thread-header **dropdown** to switch modes mid-session; switching re-renders the open transcript immediately.

## Non-goals

- User-editable / custom mode editor UI.
- App-default mode picker in Settings UI (hardcode `pretty` for v1; leave an extension point).
- Raw HTTP / ACP request inspector (reserve `rawRequests` on the mode model; always false).
- Markdown inside thinking blocks or tool I/O (stay plain / monospace).
- Syncing mode definitions themselves to the server (only the chosen mode **id** is stored on the thread).
- Changing ACP wire format, `parts[]` persistence, or LLM hydrate rules.
- Per-message mode overrides.

## Decisions

| Topic | Choice |
|-------|--------|
| Scope | Mode foundation + markdown + migrate transparency knobs into modes |
| Persistence | Thread `viewModeId` on control plane + client app default |
| Built-in presets | Differ by markdown only for v1; thinking/tools share collapsed + both |
| Naming | **Pretty** / **Detailed** |
| App default | `pretty` |
| Toggle UI | Dropdown in thread header |
| Markdown targets | Assistant answers + user messages; thinking stays plain |
| Thinking setting | Removed from Display tab; owned by mode |

## Approach

**Chosen:** Server-persisted thread mode id + client mode registry (Approach 2).

- Client owns a registry of `ViewMode` definitions (id, label, knobs).
- Thread stores optional `viewModeId`. `null` means “use app default.”
- Switching mode PATCHes the thread and updates local UI optimistically.
- Pretty uses `flutter_markdown_plus`; Detailed uses plain text as today.

**Rejected:**

- Client-only prefs map for per-thread mode — lost on reinstall / other machines.
- Flat independent toggles without a mode abstraction — fights customizeable-modes direction.

## Architecture

```text
Flutter
  ViewModeRegistry (hardcoded Pretty, Detailed)
  appDefaultModeId = "pretty"   // local pref later; hardcoded now
  Thread.viewModeId?            // from GET /v1/threads/{id}
  resolve(mode) = registry[thread.viewModeId ?? appDefault]
  ChatScreen header dropdown → PATCH /v1/threads/{id} { viewModeId }
  AgentBubble / user bubble ← ViewMode knobs (markdown, thinking, tools)

Control plane
  threads.view_mode_id TEXT NULL
  Thread JSON includes viewModeId
  PATCH validates known ids (pretty | detailed)
```

```text
resolveActiveMode(thread, appDefault, registry):
  id = thread.viewModeId ?? appDefault
  if id not in registry → appDefault
  return registry[id]
```

## Mode model

```dart
class ViewMode {
  final String id;          // "pretty" | "detailed"
  final String label;       // "Pretty" | "Detailed"
  final bool markdownRender;
  final VisibilityMode thinkingVisibility; // hidden | collapsed | expanded
  final VisibilityMode toolVisibility;     // hidden | collapsed | expanded
  final ToolIOMode toolIO;                 // input | output | both
  final bool rawRequests;                  // reserved; false in v1
}

enum ToolIOMode { input, output, both }
```

Reuse existing `VisibilityMode` from display settings (or move it next to the mode types).

### Built-in presets (v1)

| Mode id | Label | markdown | thinking | tools | tool I/O | raw |
|---------|-------|----------|----------|-------|----------|-----|
| `pretty` | Pretty | yes | collapsed | collapsed | both | no |
| `detailed` | Detailed | no | collapsed | collapsed | both | no |

Presets are equal on transparency knobs today so the toggle’s user-visible difference is Markdown. The knobs still flow through the mode object so later presets (e.g. a debug mode) can diverge without another settings migration.

## Control plane

### Schema

Migration: add nullable column

```sql
ALTER TABLE threads
  ADD COLUMN view_mode_id TEXT NULL;
```

### API

- `Thread` / list / detail JSON: `"viewModeId": "pretty" | "detailed" | null`
- `POST /v1/threads`: leave `viewModeId` null (follow app default)
- `PATCH /v1/threads/{id}`: accept optional `viewModeId`
  - Omitted field: leave stored value unchanged
  - JSON `null`: clear override (thread follows app default again)
  - `"pretty"` / `"detailed"`: set override
  - Any other string → `400`
- Listing and get return the stored value unchanged

Known ids are validated server-side against a fixed allowlist matching the client builtins for v1. When custom modes exist later, either expand the allowlist or store opaque ids with client-side fallback.

`threadPatch` must distinguish omitted `viewModeId` (no change) from explicit `null` (clear). Use a pointer or equivalent optional field in Go, not a plain `string`.

## Client behavior

### Resolution

1. Load registry (hardcoded).
2. App default = `pretty` (constant / future pref key `chat.defaultViewMode`).
3. For the selected thread, `activeMode = resolve(thread.viewModeId)`.
4. Pass `activeMode` into transcript widgets (replace `displaySettings.thinking` for thinking chrome).

### Header dropdown

- Shown when a thread is selected.
- Current label = `activeMode.label`.
- Menu: all registry modes.
- On select:
  1. Optimistic update of local thread summary/detail `viewModeId`
  2. `PATCH` with that id
  3. On failure: revert + error snackbar

Switching threads loads that thread’s stored id (or null → default). No client-side cache beyond the thread objects already held by the controller.

### Rendering

| Surface | Pretty | Detailed |
|---------|--------|----------|
| User bubble text | Markdown | Plain `Text` |
| Assistant answer (`_MessageProse`) | Markdown | Plain `Text` |
| Thinking body | Plain | Plain |
| Tool input/output | Monospace selectable | Monospace selectable |

Markdown theming should follow Material text styles (body, code, headings) and existing chat colors where applicable. The client has no `url_launcher` today: render links with markdown styling, but tapping them is a follow-up (no new dependency in this slice unless implementation finds an existing platform opener).

### Thinking & tools

- Apply `thinkingVisibility` / `toolVisibility` from the active mode the same way Display settings applied thinking today.
- While a thought or tool is **streaming**, keep starting expanded (current behavior), even if the mode prefers collapsed.
- `toolIO`: when not `both`, hide the unused Input/Output tab (both presets use `both` in v1).
- `rawRequests`: ignored in UI until a future slice.

### Settings cleanup

- Remove Thinking visibility control and preview wiring from Display tab.
- Keep content width and appearance/chat colors.
- Stop reading and writing `chat.thinkingVisibility`. Leave any stale pref value untouched; do not migrate it into modes.

## Error handling

| Case | Behavior |
|------|----------|
| PATCH network / 5xx | Revert optimistic mode; snackbar |
| PATCH 400 invalid id | Revert; snackbar |
| GET returns unknown `viewModeId` | Fall back to app default for rendering; do not crash |
| No thread selected | Hide mode dropdown |

## Testing

**Control plane**

- Migration applies; column nullable.
- Create thread → `viewModeId` null.
- PATCH set / clear / reject invalid.
- GET / list echo stored value.

**Client**

- Unit: resolve null → default; explicit override; unknown → default.
- Widget: Pretty renders markdown for user + assistant; Detailed uses plain text.
- Widget: dropdown triggers patch; failure reverts.
- Widget: thinking row respects mode visibility after Display setting removal.
- Streaming still expands in-flight thinking/tool rows.

## Future extensions (out of scope)

- Settings UI for app-default mode.
- Additional built-in **Raw** / debug mode (`rawRequests`, expanded tools/thinking).
- User-defined modes (persist definitions locally or in catalog).
- Multi-device consistency already covered for the chosen id; mode definition sync if users customize.

## Success criteria

- New threads open in Pretty with rendered Markdown for user and assistant text.
- Switching a thread to Detailed shows plain text and persists across thread switches and app restart (via control plane).
- Thinking visibility is no longer configurable in Display settings and follows the active mode.
- Mode model is ready to accept divergent transparency knobs without restructuring the thread API.
