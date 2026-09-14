# Flutter client theming and Appearance settings

**Date:** 2026-09-14  
**Status:** draft for user review  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-14-chat-part-bubbles-design.md](./2026-09-14-chat-part-bubbles-design.md)  
**Supersedes:** that spec’s non-goal “Dark theme or a full design-system rewrite; light Material, accent-bar cards only” for the Flutter client chrome and bubble colors.

## Problem

The Flutter cockpit has no app theme. `MaterialApp` uses framework defaults, and feature widgets hardcode colors (`Colors.blue`, amber/teal/gray hex in `AgentBubble`). That blocks light/dark mode, makes theming inconsistent, and forces code edits when a user wants a different thinking-block color.

Flutter 3.47 also ships standalone `material_ui` / `cupertino_ui`. Staying on in-framework Material delays the migration the ecosystem is moving toward.

## Goals

- Prefer **theme-driven styling** over per-widget hardcoded colors for chrome and chat bubbles.
- Ship **light and dark** `ThemeData` with `ThemeMode` system | light | dark (default: system).
- Expose chat-role colors via a **`ThemeExtension`** so widgets read `Theme.of(context)`, not magic hex.
- Let users tweak those role colors **in-app** from a new **Appearance** settings tab, with **separate overrides per brightness**.
- v1 color editing is **preset swatches** + reset; full color picker is a follow-up.
- Migrate the client to **`package:material_ui`** as part of this work (compatibility bridge if a dependency still needs legacy Material).
- Apply a **minor** `ColorScheme` seed tweak so primary/selection chrome coheres with today’s teal-ish answer accents — not a full rebrand.

## Non-goals

- Full color picker / hex editor (follow-up).
- Custom fonts, dense Material `*Theme` overrides beyond `ColorScheme` + `ChatColors`.
- Syncing appearance prefs to the control plane or across devices.
- Theming non-Flutter surfaces (TUI, IDE).
- Changing bubble kinds, ACP, or Thinking/Stats visibility behavior (those stay on Chat tab / `ChatDisplaySettings`).

## Approach

**Chosen:** ThemeExtension + `AppearanceSettings` (Approach 1).

- `AppTheme` builds light/dark `ThemeData` from Material 3 `ColorScheme`.
- Custom chat colors live in `ChatColors extends ThemeExtension<ChatColors>`.
- `AppearanceSettings` (`ChangeNotifier` + `shared_preferences`) owns `themeMode` and per-brightness overrides; resolves final themes for `MaterialApp`.
- Feature widgets consume theme / extension only.

**Rejected:**

- Styled wrapper widgets with injected colors only — bypasses the theme system.
- Full design-token factory — overkill for this client and slows the `material_ui` migration.

## Architecture

```text
lib/
  ui/theme/
    app_theme.dart          # light/dark ThemeData factories
    chat_colors.dart        # ThemeExtension + defaults per brightness
    color_presets.dart      # swatch lists for Appearance UI
  settings/
    appearance_settings.dart
    appearance_tab.dart
    chat_tab.dart             # unchanged concern: Thinking/Stats visibility
    settings_page.dart        # add Appearance tab
  chat/
    agent_bubble.dart         # Theme.extension<ChatColors>()
    chat_screen.dart          # user bubble from ChatColors
    thread_pane.dart          # selection from ColorScheme
  main.dart                   # material_ui MaterialApp(theme, darkTheme, themeMode)
```

**Data flow:**

1. Startup loads `AppearanceSettings` (and existing `ChatDisplaySettings`) from `SharedPreferences`.
2. `AppearanceSettings` resolves light & dark `ThemeData` = base scheme + `ChatColors` with overrides applied.
3. `MaterialApp` listens and applies `theme` / `darkTheme` / `themeMode`.
4. Chat widgets use `ColorScheme` for chrome and `Theme.extension<ChatColors>()` for bubble roles.
5. Appearance edits → persist → recompute → `notifyListeners` → themes rebuild (no hot restart).

**Migration:** depend on `material_ui`, switch imports from `package:flutter/material.dart` to `package:material_ui/material_ui.dart` (or package’s documented entrypoint). Use `MaterialUiCompatibilityBridge` in `MaterialApp.builder` only if a dependency still resolves legacy Material types.

## Components

### `ChatColors` (`ThemeExtension`)

Per brightness, roles: `thinking`, `answer`, `stats`, `user`.

Each role has:

| Property | Use |
| --- | --- |
| `fill` | Bubble background |
| `bar` | 4px left accent on agent bubbles |

All four roles expose `fill` + `bar` in the extension and Appearance UI for a uniform model. The **user** bubble widget uses `fill` only in v1 (no left bar); `user.bar` is stored/editable but unused in chat layout until a follow-up.

Caption / secondary text on answer bubbles uses `ColorScheme.onSurfaceVariant` (no separate caption color in v1).

**Light defaults:** preserve today’s palette (thinking amber, answer teal, stats gray, user blue-tint).  
**Dark defaults:** same roles with adjusted contrast (darker fills, readable bars).

Implement `copyWith` and `lerp` for theme transitions.

### `AppearanceSettings`

| Field | Values | Default |
| --- | --- | --- |
| `themeMode` | system, light, dark | system |
| overrides | map keyed by brightness + role + property → color | empty (use defaults) |

API sketch:

- `setThemeMode(ThemeMode)`
- `setRoleColor(Brightness, Role, property, Color)` — stores override
- `resetRole(Brightness, Role)` / `resetAll(Brightness)`
- `ThemeData get lightTheme` / `darkTheme` (or a single `themes` pair)

**Persistence:** keys such as `appearance.themeMode`, `appearance.light.thinking.fill`. Colors stored as ARGB `int` via `SharedPreferences`. Omit a key when the value equals the default so reset stays clean. Invalid stored values are ignored and treated as default. Prefs failures fall back to defaults and do not block the UI.

### Appearance tab

- Theme mode control (segmented button or dropdown).
- Preview strip of the four bubble roles using the resolved `ChatColors` for the **edited** brightness.
- Edited brightness: follows `themeMode` (light/dark); when system, use platform brightness from `MediaQuery` / `PlatformDispatcher`.
- Per role: fill + bar rows of preset swatches; show Reset when that role has overrides.
- Lives as a fourth Settings tab beside Providers / Agents / Chat.

### Widget refactor (in scope)

| Location | Change |
| --- | --- |
| `AgentBubble` | thinking/answer/stats fill+bar from `ChatColors` |
| `ChatScreen` user bubble | fill from `ChatColors.user` |
| `ThreadPane` selection | `ColorScheme.primary` / `primaryContainer` (or equivalent) instead of `Colors.blue` |
| Tests | assert against theme extension / scheme, not hardcoded hex |

Direct styling remains allowed for layout (padding, radius, alignment) that is not color/typography chrome.

## ColorScheme tweaks

Seed a Material 3 `ColorScheme` (light and dark) from a teal-leaning primary so NavigationRail selection and related chrome align with answer accents. Do not redesign typography or component shapes beyond what the scheme provides.

## Testing

- Unit: `ChatColors` merge/override/reset; `AppearanceSettings` round-trip with mock/`SharedPreferences.setMockInitialValues`.
- Widget: Appearance tab changes mode/swatch; bubbles resolve colors from theme; update `agent_bubble_test` color asserts.
- Smoke: app builds under light and dark after `material_ui` migration.

## Success criteria

- No feature-widget hardcoded `Color(0x…)` / `Colors.blue` for bubble or selection chrome.
- Toggling theme mode switches light/dark without restart.
- Changing a thinking fill swatch in Appearance updates chat bubbles for that brightness and survives app restart.
- Reset restores defaults for the role/brightness.
- Client runs on `material_ui` imports for Material widgets.

## Follow-ups (explicitly later)

- Full color picker / hex input.
- Optional “copy overrides to other brightness.”
- Deeper component themes / typography tokens if the cockpit grows.
