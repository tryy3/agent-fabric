import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:agent_fabric_client/ui/theme/design_tokens.dart';
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

  test('dark theme uses ChatColors.dark and the workbench palette', () {
    final theme = AppTheme.dark();
    expect(theme.brightness, Brightness.dark);
    expect(theme.colorScheme.primary, const Color(0xFF22D3D1));
    expect(theme.colorScheme.surface, const Color(0xFF0B161E));
    expect(theme.scaffoldBackgroundColor, const Color(0xFF071017));
    expect(
      theme.extension<ChatColors>()!.answer.bar,
      ChatColors.dark().answer.bar,
    );
    expect(theme.extension<DesignTokens>()!.sidebar, const Color(0xFF08131A));
  });

  test('custom chatColors override extension', () {
    final custom = ChatColors.light().withOverride(
      ChatColorRole.user,
      fill: const Color(0xFF112233),
    );
    final theme = AppTheme.light(chatColors: custom);
    expect(theme.extension<ChatColors>()!.user.fill, const Color(0xFF112233));
  });

  test('light and dark themes use bundled NotoSans', () {
    expect(
      AppTheme.light().textTheme.bodyMedium?.fontFamily,
      AppTheme.fontFamily,
    );
    expect(
      AppTheme.dark().textTheme.bodyMedium?.fontFamily,
      AppTheme.fontFamily,
    );
    expect(
      AppTheme.light().primaryTextTheme.bodyMedium?.fontFamily,
      AppTheme.fontFamily,
    );
    expect(AppTheme.monoFontFamily, 'NotoSansMono');
  });
}
