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
