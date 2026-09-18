# Advanced model picker (search + provider groups)

**Date:** 2026-09-18  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-18-chat-composer-redesign-design.md](./2026-09-18-chat-composer-redesign-design.md), [2026-09-12-dynamic-agents-providers-settings-design.md](./2026-09-12-dynamic-agents-providers-settings-design.md)

## Problem

The composer model control is a flat `DropdownButton` over session `modelOptions`. Long lists are hard to scan. Modern agent UIs offer search and provider-grouped lists in an anchored popover. Agent Fabric already has catalog providers with named model inventories; the session list can be enriched with that metadata without changing selection semantics.

## Goals

- Replace the model dropdown with a **compact chip** that opens an **anchored popover**.
- Provide **search** over model name and id.
- **Group** selectable models by **catalog provider** name (counts in headers).
- Keep selection via existing `selectModel` / session set-model (session list remains the source of truth for what can be chosen).
- Structure the popover so future top sections (favorites, configured) can plug in without a rewrite.

## Non-goals

- Custom Model ID entry.
- Configured / Selected pin section at the top of the popover.
- Favorites persistence or UI.
- Selecting models that are not in the current session `modelOptions`.
- Multi-provider session model catalogs (assumed later; V1 only enriches today’s flat list).
- Changing the agent picker to the same pattern.
- Emitting ACP `SessionConfigGroupedOptions` from the control plane (optional later).

## Decisions

| Topic | Choice |
|-------|--------|
| Approach | Client overlay + enrich from catalog |
| Data source | Session `modelOptions`; group via catalog id match |
| Presentation | Anchored popover under model chip |
| Groups | Collapsible; default expanded; no persistence beyond this open |
| Unmatched models | Group labeled **Other** |
| Catalog missing | Single **Other** group; search still works |
| Top sections | Empty extensible slot in V1 |
| Test key | Keep `model-picker` on the trigger chip |

## Approach

**Chosen:** Client-side popover that filters and groups the session model list using a `modelId → Provider` map built from catalog `listProviders()`.

**Rejected:**

- Control-plane ACP grouped options only — still one provider per agent today; less flexible for future non-agent-linked catalogs.
- Catalog-wide browser — would show non-selectable models; conflicts with session-as-source-of-truth.

## Architecture

```text
ChatComposer
  ModelPickerChip (key: model-picker)
    → opens ModelPickerPopover (Overlay / MenuAnchor)

ModelPickerPopover
  [headerSlot]          // empty V1; future favorites
  SearchField
  ListView
    ProviderGroup (expand/collapse)
      ModelRow (name, optional id subtitle, selected indicator)
        onTap → controller.selectModel(id); close

Grouping helper
  input: List<ModelOption>, List<Provider>
  output: List<ModelProviderGroup { providerId?, providerName, models }>
  match: ModelOption.id ↔ Provider.models[].id
  no match → Other
```

### Catalog loading

`ChatController` does not cache providers today. V1 should load providers once (on connect and/or when the picker first opens) via the existing `CatalogClient.listProviders()`, cache on the controller (or a dedicated small cache owned by the picker), and reuse until a later refresh path exists. Opening the popover must not spam the network every keystroke.

### Matching rules

1. Exact match on model **id**.
2. If the same id appears under multiple providers, first match wins (document; revisit with multi-provider).
3. Unmatched session models → **Other**.

### Search

- Case-insensitive substring on `ModelOption.name` and `ModelOption.id`.
- Groups with zero matches after filter are hidden.
- Collapse state is local to the open popover instance (defaults expanded when opened / when search changes optionally re-expand — prefer: keep user’s collapse toggles for the open session; new groups from filter start expanded).

## UI details

```text
┌─────────────────────────────────────┐
│  Search models…                  ✕  │
├─────────────────────────────────────┤
│  ▼ Local (12)                       │
│      Model One                   ✓  │
│      Model Two                      │
│  ▼ Other (2)                        │
│      orphan-id                      │
└─────────────────────────────────────┘
```

- Theme surfaces / outline; no nested card farm.
- Trigger shows truncated current model name (or “Model” hint) + chevron.
- Disabled when `!canSelectModel` or empty options (same rules as today).

## Testing

- Unit-test grouping/filter helper (match, Other, empty catalog, search hides groups).
- Widget-test: open popover via `model-picker`, search filters rows, select calls `selectModel`, selected row marked.
- Migrate existing tests that assume `DropdownButton` for model-picker (cast to chip / find by key).

## Implementation notes

- Prefer a dedicated widget file under `client/lib/chat/` (e.g. `model_picker.dart`) so the popover stays testable and the composer stays thin.
- Pure grouping/filter functions should be `@visibleForTesting` or top-level for unit tests without Flutter binding where possible.
- No new packages required if `MenuAnchor` / `Overlay` / `CompositedTransformFollower` from Flutter suffice.
