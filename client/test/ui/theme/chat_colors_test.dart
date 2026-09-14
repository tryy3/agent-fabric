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
