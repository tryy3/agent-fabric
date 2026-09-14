# Flutter Client Theming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Theme-driven Flutter cockpit with light/dark mode, a `ChatColors` ThemeExtension, per-brightness bubble color overrides via an Appearance settings tab (preset swatches), and migration to `package:material_ui`.

**Architecture:** `AppTheme` builds light/dark `ThemeData` (Material 3 seed + `ChatColors` extension). `AppearanceSettings` persists `ThemeMode` and override ARGB ints, merges overrides onto defaults, and exposes resolved themes to `MaterialApp`. Chat widgets read `Theme.extension<ChatColors>()` and `ColorScheme` — no hardcoded bubble/selection colors.

**Tech Stack:** Flutter (Nix shell), `package:material_ui`, `shared_preferences`, `flutter_test`. Work from repo root `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix commands with `nix develop /home/tryy3/src/agent-fabric -c` and run them from `client/`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-14-flutter-theming-design.md`](../specs/2026-09-14-flutter-theming-design.md).
- Prefer theme / `ThemeExtension` for colors; layout (padding, radius) may stay local.
- `ThemeMode` default: system. Color overrides are **per brightness**.
- v1 color UI: preset swatches + reset only (no hex picker).
- Persist colors as ARGB `int`; omit prefs keys that match defaults.
- Light bubble defaults must match today’s palette (see File Structure lock below).
- User bubble uses `fill` only in chat layout; `user.bar` is still in the extension and editable.
- Caption text uses `ColorScheme.onSurfaceVariant`, not a hardcoded gray.
- Do not change Thinking/Stats visibility keys or `ChatDisplaySettings` behavior.
- Do not commit `controlplane/data/`.
- Follow TDD: failing test → implement → pass → commit per task.
- After `material_ui` migration, Material imports are `package:material_ui/material_ui.dart` (not `package:flutter/material.dart`) in app and test sources that use Material widgets.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/pubspec.yaml` | Add `material_ui` dependency |
| `client/lib/ui/theme/chat_colors.dart` | `ChatColorRole`, `ChatColors` ThemeExtension, light/dark defaults, merge helpers |
| `client/lib/ui/theme/app_theme.dart` | `AppTheme.light` / `AppTheme.dark` factories |
| `client/lib/ui/theme/color_presets.dart` | Swatch lists for Appearance UI |
| `client/lib/settings/appearance_settings.dart` | `ThemeMode` + overrides; build themes; prefs I/O |
| `client/lib/settings/appearance_tab.dart` | Appearance Settings UI |
| `client/lib/settings/settings_page.dart` | Add Appearance tab |
| `client/lib/main.dart` | Load appearance; `MaterialApp` theme/darkTheme/themeMode; bridge if needed |
| `client/lib/chat/agent_bubble.dart` | Colors from `ChatColors` |
| `client/lib/chat/chat_screen.dart` | User bubble from `ChatColors.user.fill` |
| `client/lib/chat/thread_pane.dart` | Selection from `ColorScheme` |
| `client/test/ui/theme/chat_colors_test.dart` | Extension merge / lerp / defaults |
| `client/test/settings/appearance_settings_test.dart` | Persistence + override merge |
| `client/test/settings/appearance_tab_test.dart` | Mode + swatch widget tests |
| `client/test/chat/agent_bubble_test.dart` | Assert via theme extension defaults |
| All `client/lib/**/*.dart` and `client/test/**/*.dart` using Material | `material_ui` imports |

**Color lock (light defaults — must match current UI):**

| Role | fill | bar |
| --- | --- | --- |
| thinking | `0xFFFEF3C7` | `0xFFD97706` |
| answer | `0xFFCCFBF1` | `0xFF0F766E` |
| stats | `0xFFE4E4E7` | `0xFF71717A` |
| user | `0xFFBBDEFB` | `0xFF2196F3` |

**Dark defaults (new):**

| Role | fill | bar |
| --- | --- | --- |
| thinking | `0xFF3F2E15` | `0xFFFBBF24` |
| answer | `0xFF134E4A` | `0xFF2DD4BF` |
| stats | `0xFF3F3F46` | `0xFFA1A1AA` |
| user | `0xFF1E3A5F` | `0xFF60A5FA` |

**Seed color for `ColorScheme.fromSeed`:** `0xFF0F766E` (teal, matches answer bar).

**Interfaces this plan adds** (later tasks consume these names exactly):

```dart
enum ChatColorRole { thinking, answer, stats, user }

class RoleColors {
  const RoleColors({required this.fill, required this.bar});
  final Color fill;
  final Color bar;
  RoleColors copyWith({Color? fill, Color? bar});
}

class ChatColors extends ThemeExtension<ChatColors> {
  const ChatColors({
    required this.thinking,
    required this.answer,
    required this.stats,
    required this.user,
  });
  final RoleColors thinking;
  final RoleColors answer;
  final RoleColors stats;
  final RoleColors user;

  static ChatColors light();
  static ChatColors dark();
  RoleColors forRole(ChatColorRole role);
  ChatColors withOverride(ChatColorRole role, {Color? fill, Color? bar});
  // copyWith + lerp required by ThemeExtension
}

class AppTheme {
  static ThemeData light({ChatColors? chatColors});
  static ThemeData dark({ChatColors? chatColors});
}

class AppearanceSettings extends ChangeNotifier {
  static Future<AppearanceSettings> load();
  ThemeMode themeMode;
  ThemeData get lightTheme;
  ThemeData get darkTheme;
  ChatColors colorsFor(Brightness brightness);
  bool hasOverride(Brightness brightness, ChatColorRole role);
  Future<void> setThemeMode(ThemeMode mode);
  Future<void> setRoleColor(
    Brightness brightness,
    ChatColorRole role, {
    Color? fill,
    Color? bar,
  });
  Future<void> resetRole(Brightness brightness, ChatColorRole role);
  Future<void> resetAll(Brightness brightness);
}
```

---

### Task 1: Migrate client to `material_ui`

**Files:**
- Modify: `client/pubspec.yaml`
- Modify: every `client/lib/**/*.dart` and `client/test/**/*.dart` that imports `package:flutter/material.dart`
- Modify: `client/lib/main.dart` (add compatibility bridge)

**Interfaces:**
- Consumes: none from later tasks
- Produces: app/tests import `package:material_ui/material_ui.dart`; `MaterialApp` wraps child with `MaterialUiCompatibilityBridge` when needed

- [ ] **Step 1: Add dependency**

From `client/`:

```bash
nix develop /home/tryy3/src/agent-fabric -c flutter pub add material_ui
```

Expected: `pubspec.yaml` lists `material_ui`, `flutter pub get` succeeds.

- [ ] **Step 2: Migrate imports**

```bash
nix develop /home/tryy3/src/agent-fabric -c dart fix --apply --code=migrate_design_widgets
```

If the fix code is unavailable on this SDK, manually replace in all lib/test Dart files:

```dart
// before
import 'package:flutter/material.dart';
// after
import 'package:material_ui/material_ui.dart';
```

Keep `package:flutter/foundation.dart` (and other non-Material flutter imports) as-is where already used (e.g. `display_settings.dart`).

- [ ] **Step 3: Wire compatibility bridge in `main.dart`**

In `AgentFabricApp.build`, change `MaterialApp` to:

```dart
return MaterialApp(
  title: 'Agent Fabric',
  builder: (context, child) {
    return MaterialUiCompatibilityBridge(child: child!);
  },
  home: AppShell(
    controller: _controller,
    catalog: _catalog,
    displaySettings: widget.displaySettings,
  ),
);
```

- [ ] **Step 4: Run full client tests**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test
```

Expected: PASS (or only pre-existing failures unrelated to imports). Fix any type/import breakages before continuing.

- [ ] **Step 5: Commit**

```bash
git add client/pubspec.yaml client/pubspec.lock client/lib client/test
git commit -m "$(cat <<'EOF'
chore(client): migrate Material widgets to material_ui

EOF
)"
```

---

### Task 2: `ChatColors` ThemeExtension + defaults

**Files:**
- Create: `client/lib/ui/theme/chat_colors.dart`
- Create: `client/test/ui/theme/chat_colors_test.dart`

**Interfaces:**
- Consumes: `material_ui` `Color`, `ThemeExtension`
- Produces: `ChatColorRole`, `RoleColors`, `ChatColors` with `light()`, `dark()`, `forRole`, `withOverride`, `copyWith`, `lerp`

- [ ] **Step 1: Write failing tests**

Create `client/test/ui/theme/chat_colors_test.dart`:

```dart
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  test('light defaults match current bubble palette', () {
    final c = ChatColors.light();
    expect(c.thinking.fill, const Color(0xFFFEF3C7));
    expect(c.thinking.bar, const Color(0xFFD97706));
    expect(c.answer.fill, const Color(0xFFCCFBF1));
    expect(c.answer.bar, const Color(0xFF0F766E));
    expect(c.stats.fill, const Color(0xFFE4E4E7));
    expect(c.stats.bar, const Color(0xFF71717A));
    expect(c.user.fill, const Color(0xFFBBDEFB));
    expect(c.user.bar, const Color(0xFF2196F3));
  });

  test('dark defaults use distinct darker fills', () {
    final c = ChatColors.dark();
    expect(c.thinking.fill, const Color(0xFF3F2E15));
    expect(c.thinking.bar, const Color(0xFFFBBF24));
    expect(c.answer.fill, const Color(0xFF134E4A));
    expect(c.answer.bar, const Color(0xFF2DD4BF));
    expect(c.stats.fill, const Color(0xFF3F3F46));
    expect(c.stats.bar, const Color(0xFFA1A1AA));
    expect(c.user.fill, const Color(0xFF1E3A5F));
    expect(c.user.bar, const Color(0xFF60A5FA));
  });

  test('withOverride changes only the given role property', () {
    final c = ChatColors.light().withOverride(
      ChatColorRole.thinking,
      fill: const Color(0xFFFF0000),
    );
    expect(c.thinking.fill, const Color(0xFFFF0000));
    expect(c.thinking.bar, const Color(0xFFD97706));
    expect(c.answer.fill, const Color(0xFFCCFBF1));
  });

  test('forRole returns the matching RoleColors', () {
    final c = ChatColors.light();
    expect(c.forRole(ChatColorRole.stats).bar, const Color(0xFF71717A));
  });

  test('lerp at 0 and 1 returns endpoints', () {
    final a = ChatColors.light();
    final b = ChatColors.dark();
    expect(a.lerp(b, 0)!.thinking.fill, a.thinking.fill);
    expect(a.lerp(b, 1)!.thinking.fill, b.thinking.fill);
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/ui/theme/chat_colors_test.dart
```

Expected: FAIL — `chat_colors.dart` not found / undefined classes.

- [ ] **Step 3: Implement `chat_colors.dart`**

Create `client/lib/ui/theme/chat_colors.dart` implementing the interfaces above with the locked light/dark color tables. `withOverride` must copy other roles unchanged. `lerp` must lerp each role’s fill and bar via `Color.lerp`.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/ui/theme/chat_colors_test.dart
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/ui/theme/chat_colors.dart client/test/ui/theme/chat_colors_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add ChatColors ThemeExtension with light/dark defaults

EOF
)"
```

---

### Task 3: `AppTheme` factories

**Files:**
- Create: `client/lib/ui/theme/app_theme.dart`
- Create: `client/test/ui/theme/app_theme_test.dart`

**Interfaces:**
- Consumes: `ChatColors.light()`, `ChatColors.dark()`
- Produces: `AppTheme.light({ChatColors? chatColors})`, `AppTheme.dark({ChatColors? chatColors})`

- [ ] **Step 1: Write failing tests**

```dart
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  test('light theme uses seed and ChatColors.light by default', () {
    final theme = AppTheme.light();
    expect(theme.brightness, Brightness.light);
    expect(theme.colorScheme.primary, isNot(equals(const Color(0xFF000000))));
    expect(
      theme.extension<ChatColors>()!.thinking.fill,
      ChatColors.light().thinking.fill,
    );
  });

  test('dark theme uses ChatColors.dark by default', () {
    final theme = AppTheme.dark();
    expect(theme.brightness, Brightness.dark);
    expect(
      theme.extension<ChatColors>()!.answer.bar,
      ChatColors.dark().answer.bar,
    );
  });

  test('custom chatColors override extension', () {
    final custom = ChatColors.light().withOverride(
      ChatColorRole.user,
      fill: const Color(0xFF112233),
    );
    final theme = AppTheme.light(chatColors: custom);
    expect(theme.extension<ChatColors>()!.user.fill, const Color(0xFF112233));
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/ui/theme/app_theme_test.dart
```

Expected: FAIL — missing `app_theme.dart`.

- [ ] **Step 3: Implement `app_theme.dart`**

```dart
import 'package:material_ui/material_ui.dart';

import 'chat_colors.dart';

abstract final class AppTheme {
  static const seed = Color(0xFF0F766E);

  static ThemeData light({ChatColors? chatColors}) {
    final scheme = ColorScheme.fromSeed(
      seedColor: seed,
      brightness: Brightness.light,
    );
    return ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      extensions: [chatColors ?? ChatColors.light()],
    );
  }

  static ThemeData dark({ChatColors? chatColors}) {
    final scheme = ColorScheme.fromSeed(
      seedColor: seed,
      brightness: Brightness.dark,
    );
    return ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      extensions: [chatColors ?? ChatColors.dark()],
    );
  }
}
```

If `useMaterial3` is removed/renamed in `material_ui`, omit it and rely on package defaults — keep seed + extensions.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/ui/theme/app_theme_test.dart
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/ui/theme/app_theme.dart client/test/ui/theme/app_theme_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add AppTheme light/dark factories

EOF
)"
```

---

### Task 4: `AppearanceSettings` persistence and theme resolution

**Files:**
- Create: `client/lib/settings/appearance_settings.dart`
- Create: `client/test/settings/appearance_settings_test.dart`

**Interfaces:**
- Consumes: `AppTheme`, `ChatColors`, `ChatColorRole`
- Produces: `AppearanceSettings` API listed in File Structure

**Persistence keys:**
- `appearance.themeMode` → `system` | `light` | `dark`
- `appearance.{light|dark}.{thinking|answer|stats|user}.{fill|bar}` → ARGB `int`

- [ ] **Step 1: Write failing tests**

```dart
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
  });

  test('defaults to system mode and default chat colors', () async {
    final a = await AppearanceSettings.load();
    expect(a.themeMode, ThemeMode.system);
    expect(
      a.colorsFor(Brightness.light).thinking.fill,
      ChatColors.light().thinking.fill,
    );
    expect(
      a.colorsFor(Brightness.dark).thinking.fill,
      ChatColors.dark().thinking.fill,
    );
  });

  test('setThemeMode persists', () async {
    final a = await AppearanceSettings.load();
    await a.setThemeMode(ThemeMode.dark);
    final a2 = await AppearanceSettings.load();
    expect(a2.themeMode, ThemeMode.dark);
  });

  test('setRoleColor overrides one property and persists', () async {
    final a = await AppearanceSettings.load();
    await a.setRoleColor(
      Brightness.light,
      ChatColorRole.thinking,
      fill: const Color(0xFFFF00FF),
    );
    expect(a.hasOverride(Brightness.light, ChatColorRole.thinking), isTrue);
    expect(
      a.colorsFor(Brightness.light).thinking.fill,
      const Color(0xFFFF00FF),
    );
    expect(
      a.colorsFor(Brightness.light).thinking.bar,
      ChatColors.light().thinking.bar,
    );

    final a2 = await AppearanceSettings.load();
    expect(
      a2.colorsFor(Brightness.light).thinking.fill,
      const Color(0xFFFF00FF),
    );
  });

  test('resetRole clears overrides for that role/brightness', () async {
    final a = await AppearanceSettings.load();
    await a.setRoleColor(
      Brightness.dark,
      ChatColorRole.answer,
      bar: const Color(0xFF00FF00),
    );
    await a.resetRole(Brightness.dark, ChatColorRole.answer);
    expect(a.hasOverride(Brightness.dark, ChatColorRole.answer), isFalse);
    expect(
      a.colorsFor(Brightness.dark).answer.bar,
      ChatColors.dark().answer.bar,
    );
  });

  test('lightTheme and darkTheme expose ChatColors extension', () async {
    final a = await AppearanceSettings.load();
    expect(a.lightTheme.extension<ChatColors>(), isNotNull);
    expect(a.darkTheme.extension<ChatColors>(), isNotNull);
  });

  test('setting color equal to default removes persisted key', () async {
    SharedPreferences.setMockInitialValues({
      'appearance.light.thinking.fill': const Color(0xFFFF0000).toARGB32(),
    });
    // If toARGB32 is unavailable, use `.value` / package equivalent for ARGB int.
    final a = await AppearanceSettings.load();
    await a.setRoleColor(
      Brightness.light,
      ChatColorRole.thinking,
      fill: ChatColors.light().thinking.fill,
    );
    final prefs = await SharedPreferences.getInstance();
    expect(prefs.containsKey('appearance.light.thinking.fill'), isFalse);
  });
}
```

Note: use whatever ARGB int API `material_ui`/`dart:ui` Color exposes on this SDK (`toARGB32()`, or `value` if still present). Be consistent in production code and tests.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/settings/appearance_settings_test.dart
```

Expected: FAIL — missing `appearance_settings.dart`.

- [ ] **Step 3: Implement `AppearanceSettings`**

Implement load/parse/save. On load, skip invalid ints. Build `ChatColors` by starting from `ChatColors.light()` / `dark()` and applying each stored override via `withOverride`. `lightTheme` / `darkTheme` call `AppTheme.light(chatColors: …)` / `dark(…)`. `notifyListeners()` after every successful mutation.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/settings/appearance_settings_test.dart
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/settings/appearance_settings.dart client/test/settings/appearance_settings_test.dart
git commit -m "$(cat <<'EOF'
feat(client): persist AppearanceSettings theme mode and color overrides

EOF
)"
```

---

### Task 5: Wire themes into `MaterialApp`

**Files:**
- Modify: `client/lib/main.dart`
- Modify: `client/test/widget_test.dart` (and any test that constructs `AgentFabricApp` if themes are required)

**Interfaces:**
- Consumes: `AppearanceSettings.load()`, `lightTheme`, `darkTheme`, `themeMode`
- Produces: running app respects theme mode

- [ ] **Step 1: Write failing widget test**

Add to `client/test/widget_test.dart` (or a small new `client/test/app_theme_wiring_test.dart`):

```dart
testWidgets('MaterialApp uses AppearanceSettings themes', (tester) async {
  SharedPreferences.setMockInitialValues({});
  final display = await ChatDisplaySettings.load();
  final appearance = await AppearanceSettings.load();
  await appearance.setThemeMode(ThemeMode.dark);

  await tester.pumpWidget(
    AgentFabricApp(
      displaySettings: display,
      appearanceSettings: appearance,
      // pass existing optional controller/catalog stubs as other tests do
    ),
  );

  final app = tester.widget<MaterialApp>(find.byType(MaterialApp));
  expect(app.themeMode, ThemeMode.dark);
  expect(app.theme?.extension<ChatColors>(), isNotNull);
  expect(app.darkTheme?.extension<ChatColors>(), isNotNull);
});
```

Adjust constructor args to match how `AgentFabricApp` is extended in Step 3.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/widget_test.dart
```

Expected: FAIL — no `appearanceSettings` parameter / themes unset.

- [ ] **Step 3: Implement wiring**

In `main()`:

```dart
final displaySettings = await ChatDisplaySettings.load();
final appearanceSettings = await AppearanceSettings.load();
runApp(AgentFabricApp(
  displaySettings: displaySettings,
  appearanceSettings: appearanceSettings,
));
```

Add `final AppearanceSettings appearanceSettings` to `AgentFabricApp`. In `build`:

```dart
return ListenableBuilder(
  listenable: widget.appearanceSettings,
  builder: (context, _) {
    final appearance = widget.appearanceSettings;
    return MaterialApp(
      title: 'Agent Fabric',
      theme: appearance.lightTheme,
      darkTheme: appearance.darkTheme,
      themeMode: appearance.themeMode,
      builder: (context, child) {
        return MaterialUiCompatibilityBridge(child: child!);
      },
      home: AppShell(
        controller: _controller,
        catalog: _catalog,
        displaySettings: widget.displaySettings,
        appearanceSettings: appearance,
      ),
    );
  },
);
```

Thread `appearanceSettings` through `AppShell` → `SettingsPage` (required for Task 7). For Task 5 it is enough that `AppShell` accepts and holds the field even if Settings does not use it yet.

Update existing tests that construct `AgentFabricApp` / `AppShell` / `SettingsPage` to load and pass `AppearanceSettings`.

- [ ] **Step 4: Run affected tests**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/main.dart client/lib/app_shell.dart client/test
git commit -m "$(cat <<'EOF'
feat(client): wire AppearanceSettings into MaterialApp themes

EOF
)"
```

---

### Task 6: Refactor chat widgets to theme colors

**Files:**
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/lib/chat/thread_pane.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`

**Interfaces:**
- Consumes: `Theme.of(context).extension<ChatColors>()!`, `Theme.of(context).colorScheme`
- Produces: no hardcoded bubble/selection colors in those widgets

- [ ] **Step 1: Update failing color assertions first**

In `agent_bubble_test.dart`, wrap pumps with themed `MaterialApp`:

```dart
MaterialApp(
  theme: AppTheme.light(),
  home: Scaffold(body: AgentBubble(...)),
)
```

Change color expects to:

```dart
final colors = ChatColors.light();
expect(thought.color, colors.thinking.fill);
expect(
  thought.border,
  Border(left: BorderSide(color: colors.thinking.bar, width: 4)),
);
// same pattern for answer + stats
```

Also update caption color assertion if any test checks the gray caption — expect `AppTheme.light().colorScheme.onSurfaceVariant` (or read from pumped theme).

- [ ] **Step 2: Run bubble tests — they should fail on implementation still using hex OR pass if values coincide; force failure by temporarily expecting theme path**

Run:

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/agent_bubble_test.dart
```

If tests still pass against hardcoded hex (same values), proceed to Step 3 and then add a regression test that overrides extension:

```dart
testWidgets('AgentBubble uses ChatColors from theme extension', (tester) async {
  final custom = ChatColors.light().withOverride(
    ChatColorRole.thinking,
    fill: const Color(0xFFABCDEF),
    bar: const Color(0xFF123456),
  );
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(chatColors: custom),
      home: const Scaffold(
        body: AgentBubble(
          bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 't'),
          thinkingMode: VisibilityMode.expanded,
        ),
      ),
    ),
  );
  final box = tester.widget<Container>(/* same ancestor finder as existing test */);
  final deco = box.decoration! as BoxDecoration;
  expect(deco.color, const Color(0xFFABCDEF));
});
```

Run until this new test fails (widget still uses hardcoded amber).

- [ ] **Step 3: Implement widget refactor**

`agent_bubble.dart` — at start of each role builder:

```dart
final chat = Theme.of(context).extension<ChatColors>()!;
// _thought / _message / _stats use chat.thinking / answer / stats
// caption:
style: Theme.of(context).textTheme.bodySmall?.copyWith(
  color: Theme.of(context).colorScheme.onSurfaceVariant,
),
```

Pass `BuildContext` into `_thought` / `_stats` (they currently omit it).

`chat_screen.dart` user bubble:

```dart
color: Theme.of(context).extension<ChatColors>()!.user.fill,
```

`thread_pane.dart`:

```dart
final scheme = Theme.of(context).colorScheme;
selectedTileColor: scheme.primaryContainer,
shape: Border(
  left: BorderSide(
    color: selected ? scheme.primary : Colors.transparent,
    width: 3,
  ),
),
```

Prefer `scheme.surface.withValues(alpha: 0)` or `BorderSide(color: selected ? scheme.primary : scheme.surface.withOpacity(0))` only if `Colors.transparent` is undesirable; `Colors.transparent` for a non-chrome edge case is acceptable. Do **not** use `Colors.blue`.

- [ ] **Step 4: Run chat UI tests**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/lib/chat/chat_screen.dart client/lib/chat/thread_pane.dart client/test/chat
git commit -m "$(cat <<'EOF'
refactor(client): drive bubble and selection colors from theme

EOF
)"
```

---

### Task 7: Appearance tab + presets + Settings integration

**Files:**
- Create: `client/lib/ui/theme/color_presets.dart`
- Create: `client/lib/settings/appearance_tab.dart`
- Modify: `client/lib/settings/settings_page.dart`
- Modify: `client/lib/app_shell.dart` (pass appearance into Settings)
- Create: `client/test/settings/appearance_tab_test.dart`

**Interfaces:**
- Consumes: `AppearanceSettings`, `ChatColorRole`, `color_presets`
- Produces: Settings → Appearance tab; user can set mode and swatches

- [ ] **Step 1: Define presets**

Create `client/lib/ui/theme/color_presets.dart`:

```dart
import 'package:material_ui/material_ui.dart';

const kFillPresets = <Color>[
  Color(0xFFFEF3C7),
  Color(0xFFCCFBF1),
  Color(0xFFE4E4E7),
  Color(0xFFBBDEFB),
  Color(0xFFFCE7F3),
  Color(0xFFE0E7FF),
  Color(0xFF3F2E15),
  Color(0xFF134E4A),
  Color(0xFF3F3F46),
  Color(0xFF1E3A5F),
];

const kBarPresets = <Color>[
  Color(0xFFD97706),
  Color(0xFF0F766E),
  Color(0xFF71717A),
  Color(0xFF2196F3),
  Color(0xFFDB2777),
  Color(0xFF4F46E5),
  Color(0xFFFBBF24),
  Color(0xFF2DD4BF),
  Color(0xFFA1A1AA),
  Color(0xFF60A5FA),
];
```

- [ ] **Step 2: Write failing Appearance tab tests**

```dart
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/appearance_tab.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() => SharedPreferences.setMockInitialValues({}));

  testWidgets('theme mode control updates AppearanceSettings', (tester) async {
    final appearance = await AppearanceSettings.load();
    await tester.pumpWidget(
      MaterialApp(home: AppearanceTab(settings: appearance)),
    );
    await tester.tap(find.byKey(const Key('theme-mode')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Dark').last);
    await tester.pumpAndSettle();
    expect(appearance.themeMode, ThemeMode.dark);
  });

  testWidgets('tapping a fill swatch overrides thinking fill', (tester) async {
    final appearance = await AppearanceSettings.load();
    await tester.pumpWidget(
      MaterialApp(
        theme: appearance.lightTheme,
        home: AppearanceTab(settings: appearance),
      ),
    );
    await tester.tap(find.byKey(const Key('swatch-thinking-fill-0')));
    await tester.pumpAndSettle();
    expect(
      appearance.hasOverride(Brightness.light, ChatColorRole.thinking),
      isTrue,
    );
  });

  testWidgets('Settings page shows Appearance tab', (tester) async {
    final display = await ChatDisplaySettings.load();
    final appearance = await AppearanceSettings.load();
    final catalog = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient(
        (_) async => http.Response(
          '[]',
          200,
          headers: {'content-type': 'application/json'},
        ),
      ),
    );
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: display,
          appearanceSettings: appearance,
        ),
      ),
    );
    expect(find.text('Appearance'), findsOneWidget);
  });
}
```

- [ ] **Step 3: Run tests — expect FAIL**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/settings/appearance_tab_test.dart
```

- [ ] **Step 4: Implement `AppearanceTab` and wire Settings**

`AppearanceTab`:
- `ListenableBuilder` on `settings`
- Theme mode `DropdownButtonFormField` or `SegmentedButton` with key `theme-mode`, items System / Light / Dark
- Determine edit brightness: `themeMode == ThemeMode.light` → light; `dark` → dark; `system` → `MediaQuery.platformBrightnessOf(context)`
- Preview row: four small containers using `settings.colorsFor(brightness)` fills/bars
- For each `ChatColorRole`: section title, fill swatch row, bar swatch row
- Each swatch: `InkWell` / `GestureDetector` with key `swatch-{role}-fill-{index}` / `swatch-{role}-bar-{index}` calling `setRoleColor`
- Reset button per role when `hasOverride` is true → `resetRole`

`SettingsPage`: `DefaultTabController(length: 4)`, add `Tab(text: 'Appearance')` and `AppearanceTab(settings: appearanceSettings)`.

Pass `appearanceSettings` from `AppShell`.

- [ ] **Step 5: Run full test suite**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add client/lib/ui/theme/color_presets.dart client/lib/settings/appearance_tab.dart client/lib/settings/settings_page.dart client/lib/app_shell.dart client/test/settings/appearance_tab_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add Appearance settings tab with theme mode and color swatches

EOF
)"
```

---

### Task 8: Smoke verification + README note

**Files:**
- Modify: `client/README.md` (short Appearance / theme note) only if the file already documents Settings tabs
- No new production code unless smoke reveals gaps

- [ ] **Step 1: Full test suite**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test
```

Expected: all PASS

- [ ] **Step 2: Analyze**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c dart analyze
```

Expected: no issues in files touched by this plan.

- [ ] **Step 3: Grep for leftover hardcoded chrome colors**

```bash
rg "Colors\\.blue|0xFFD97706|0xFFFEF3C7|0xFF0F766E|0xFFCCFBF1" client/lib --glob '*.dart'
```

Expected: hits only inside `client/lib/ui/theme/` (defaults/presets), not in `chat/` or `settings/` feature widgets (except keys/comments).

- [ ] **Step 4: Commit any README / leftover fixes**

```bash
git add client/README.md
git commit -m "$(cat <<'EOF'
docs(client): note Appearance theme settings

EOF
)"
```

Skip this commit if README unchanged.

---

## Spec coverage self-check

| Spec requirement | Task |
| --- | --- |
| Prefer theme over hardcoded colors | 6, 8 |
| Light/dark + ThemeMode system/light/dark | 3, 4, 5, 7 |
| `ChatColors` ThemeExtension | 2 |
| Appearance tab, per-brightness overrides, swatches + reset | 4, 7 |
| Persist ARGB ints; omit default keys | 4 |
| `material_ui` migration + bridge | 1, 5 |
| Minor ColorScheme seed tweak | 3 |
| Caption via onSurfaceVariant | 6 |
| User fill only in layout; bar still in model | 2, 6, 7 |
| Update bubble tests | 6 |
| No ChatDisplaySettings / ACP changes | — respected |

## Placeholder / consistency notes

- Color ARGB API name (`toARGB32` vs `value`) must be chosen once against the installed SDK and used everywhere in Task 4.
- All Material imports after Task 1: `package:material_ui/material_ui.dart`.
- Interface names in Tasks 5–7 match the File Structure block exactly (`AppearanceSettings`, `ChatColorRole`, `colorsFor`, `setRoleColor`, …).
