# Catalog provider and agent management (edit + delete)

**Date:** 2026-09-13  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-12-dynamic-agents-providers-settings-design.md](./2026-09-12-dynamic-agents-providers-settings-design.md)

## Problem

The catalog HTTP API and Flutter `CatalogClient` already support PATCH and DELETE for providers and agents. Settings only exposes create (providers) and create plus tap-to-edit (agents). There is no way to change a provider or delete either resource from the UI.

Provider delete currently returns `409` if any agent still references it. That blocks cleanup unless the user deletes those agents first.

## Goals

- Settings: **edit and delete** providers and agents.
- Tap a list row to edit (providers match the existing agent editor).
- Delete via a per-row icon, with a confirmation dialog that names the resource.
- Deleting a provider **unlinks** referencing agents (`providerId` and `defaultModel` unset) instead of `409`.
- Chat lists incomplete agents but does not let the user select them. Incomplete items are disabled and labeled so the problem is visible before send.
- If the currently selected chat agent becomes incomplete or missing, keep it selected, block send, and show the same needs-provider (or gone) state.
- Refresh the Chat agent list when returning to Chat from Settings.

## Non-goals

- Cascade-delete agents when deleting a provider
- Creating an agent without a provider and model
- Encrypting API keys, catalog auth, or multi-user
- Mid-session agent switching
- Changing provider `type` (stays `openai_compatible`)
- Auto-refreshing models after a provider URL/key edit
- Undo-delete / snackbar undo
- Pagination of catalog lists

## Approach

**Chosen:** Server unlink on provider delete (Approach 1).

- `DELETE /v1/providers/{id}` unsets `providerId` and `defaultModel` on referencing agents (bump `version`), persists `agents.json`, then removes the provider and persists `providers.json`.
- Settings confirms first and names affected agents. One HTTP delete; the catalog is the source of truth.
- Create still requires a valid provider + cached model. Only provider-delete leaves those fields unset. Repair is edit.

**Rejected:**

- Client-orchestrated PATCH-then-DELETE with leftover `409` — racy, two-step, duplicates policy in the UI.
- Dangling `providerId` strings after delete — agents would point at missing records.

## Architecture

```text
Settings ──HTTP──► Catalog store (JSON)
  tap row     → PATCH /v1/providers/{id} | PATCH /v1/agents/{id}
  delete      → confirm → DELETE
  DELETE /v1/providers/{id}
      1. unset providerId + defaultModel on referencing agents (bump version)
      2. persist agents.json, then providers.json
      3. remove the provider

Chat ──GET /v1/agents──► same catalog
  complete agent   → selectable → session/new
  incomplete agent → listed, disabled, labeled “needs provider”
  selected + incomplete or missing → keep selection, block send (no session/new)

ACP ── session/new still requires a complete agent (server fail-safe)
```

`409 Provider in use` is removed. Unknown id stays `404`.

Chat currently loads agents only on connect. After Settings changes, Chat refreshes `GET /v1/agents` when the Chat destination becomes visible again.

## Data model

`Agent.providerId` and `Agent.defaultModel` become optional. JSON uses `null` when unset (`*string` in Go, `String?` in Dart — the field is present as `null`, not `""`).

| Field | Create | After provider delete | Repair (PATCH) |
| --- | --- | --- | --- |
| `providerId` | required, existing provider | `null` | must be set together with `defaultModel` |
| `defaultModel` | required, in that provider’s cached models | `null` | must be set together with `providerId` |

An agent is **incomplete** when `providerId` or `defaultModel` is null or empty. Completeness is a client convenience; the store also treats null as incomplete.

Unlink bumps `version` and `updatedAt` on each affected agent.

## Catalog HTTP API

Existing paths. Behavior changes:

| Method | Path | Change |
| --- | --- | --- |
| `GET` | `/v1/agents` | Incomplete agents included; `providerId` / `defaultModel` may be `null` |
| `POST` | `/v1/agents` | Unchanged: both fields required and valid |
| `PATCH` | `/v1/agents/{id}` | If either provider or model is sent, both must be non-null and valid. Pair must not be half-set. |
| `DELETE` | `/v1/agents/{id}` | Unchanged: `204` or `404` |
| `PATCH` | `/v1/providers/{id}` | Unchanged: name, baseUrl, apiKey |
| `DELETE` | `/v1/providers/{id}` | Unlink then delete. `204` or `404`. **No `409`.** |

The client does not PATCH agents to null; unlink happens only inside `DeleteProvider`.

Persist unlink + delete as one store operation under the existing mutex: write `agents.json` first, then `providers.json`. If the process stops after agents are saved, the provider still exists and agents are unset (repairable). The reverse order would leave dangling ids.

## Components

**Catalog store / HTTP**

- Drop `ErrProviderInUse` and the HTTP `409` mapping.
- `DeleteProvider` unlinks, then deletes.
- `CreateAgent` still calls `validateProviderAndModel`.
- `UpdateAgent` on the resulting fields: both unset → skip model validation (name/description-only patch of an incomplete agent). One set and the other unset → `400` (“provider and model must be set together”). Both set → `validateProviderAndModel`. The Settings editor still requires both before Save, so the UI cannot submit a half-set pair.

**Flutter catalog client / models**

- `Agent.providerId` and `defaultModel` become `String?`.
- Existing `updateProvider` / `deleteProvider` / `updateAgent` / `deleteAgent` are sufficient.

**Settings — Providers**

- Tap a row to open an editor (name, base URL, API key; type fixed). Save → PATCH, then reload.
- Trailing **Refresh models** stays.
- Delete icon on each row. Confirm with the provider name. On delete, fetch agents (`GET /v1/agents` if this tab does not already have them) and list names that still reference this provider; the copy says deleting will unset their provider and model. The server unlinks even if that list is stale.

**Settings — Agents**

- Tap-to-edit unchanged.
- Same delete icon + confirm (agent name only).
- Incomplete agents: subtitle **Needs provider**.

**Chat**

- Picker lists every agent. Incomplete `DropdownMenuItem`s are `enabled: false` and labeled, e.g. `Work — needs provider`.
- `canSend` requires a complete selected agent (in addition to connected / not sending / session ready).
- If the current selection is incomplete: keep `selectedAgentId`, do not call `session/new`, disable the composer, show the needs-provider message.
- Refresh agents when Chat becomes visible (sidebar back from Settings), not only on first `connect()`.

## Data flow

**Edit.** Tap row → prefilled dialog → PATCH → reload that tab.

**Delete agent.** Icon → confirm name → `DELETE /v1/agents/{id}` → drop from the Settings list.

**Delete provider.** Icon → confirm, naming referencing agents from the in-memory (or reloaded) agent list → `DELETE /v1/providers/{id}` → reload providers. Agents tab and Chat see unset fields on their next load.

**Chat after Settings.** On becoming visible, `listAgents()`, then:

1. Recompute completeness from `providerId` / `defaultModel`.
2. If `selectedAgentId` is still a complete agent → leave the ACP session as-is (no automatic `session/new`).
3. If selected is incomplete → do not call `session/new`; `_sessionReady` false; `canSend` false; needs-provider message.
4. If selected id is absent from the list → keep `selectedAgentId`; show a disabled leftover in the picker (e.g. `(deleted)`); block send until the user picks a complete agent.
5. Picking a complete agent starts a new session as today (clears transcript).

**Repair.** Edit the incomplete agent, choose provider + model, Save. After the next Chat refresh, that row is selectable.

## Error handling

| Case | Behavior |
| --- | --- |
| PATCH/DELETE unknown id | `404`; Settings shows the catalog error on the tab |
| Create/update agent with missing or unmatched provider/model | `400`; editor stays open with the message |
| Update agent with only provider or only model set | `400` (“provider and model must be set together”) |
| Delete provider | Always unlinks then deletes; no `409` |
| Confirm dialog cancelled | No request |
| Delete/PATCH network or other catalog error | Keep the row; show the error on the tab (same as refresh-models failures) |
| Chat `session/new` on an incomplete agent | UI must not send this. ACP still errors if it happens; composer stays blocked |
| Selected agent incomplete after refresh | No `session/new`; disable send; visible “needs provider” |
| Selected agent missing after delete | Same, disabled leftover in the picker until another agent is chosen |
| Provider editor empty name / URL | Disable Save or surface `400` in the dialog |

## Testing

**Go (catalog store + HTTP)**

- `DeleteProvider` with referencing agents: those agents get null `providerId`/`defaultModel`, version bumped, provider gone; both JSON files round-trip.
- `DeleteProvider` with no refs: provider gone, agents unchanged.
- `DeleteProvider` unknown id: error / HTTP `404`.
- No `409` / `ErrProviderInUse` on delete.
- Create still rejects missing provider/model.
- Update name-only on an incomplete agent: succeeds; fields stay null.
- Update with only one of provider/model set: `400`.
- `session/new` with an incomplete agent: JSON-RPC error (existing fail path).

**Flutter**

- Catalog client parses `providerId`/`defaultModel` as null.
- Providers tab: tap row opens editor; save calls PATCH; delete icon → confirm → DELETE; in-use confirm lists agent names.
- Agents tab: delete icon → confirm → DELETE; incomplete agent shows “Needs provider”.
- Chat picker: incomplete item disabled and labeled; `canSend` is false when the selected agent is incomplete; returning to Chat reloads agents.

## Follow-ups

- Cascade-delete agents (explicit choice, not this slice)
- Encrypt secrets at rest; catalog auth
- Auto-refresh models after provider URL/key change
- Default-agent setting so Chat is not stuck on a leftover deleted selection
