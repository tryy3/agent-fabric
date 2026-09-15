import 'package:material_ui/material_ui.dart';

enum ChatColorRole { thinking, answer, stats, user }

class RoleColors {
  const RoleColors({required this.fill, required this.bar});

  final Color fill;
  final Color bar;

  RoleColors copyWith({Color? fill, Color? bar}) {
    return RoleColors(fill: fill ?? this.fill, bar: bar ?? this.bar);
  }

  static RoleColors lerp(RoleColors a, RoleColors b, double t) {
    return RoleColors(
      fill: Color.lerp(a.fill, b.fill, t)!,
      bar: Color.lerp(a.bar, b.bar, t)!,
    );
  }
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

  factory ChatColors.light() {
    return const ChatColors(
      thinking: RoleColors(fill: Color(0xFFFEF3C7), bar: Color(0xFFD97706)),
      answer: RoleColors(fill: Color(0xFFCCFBF1), bar: Color(0xFF0F766E)),
      stats: RoleColors(fill: Color(0xFFE4E4E7), bar: Color(0xFF71717A)),
      user: RoleColors(fill: Color(0xFFBBDEFB), bar: Color(0xFF2196F3)),
    );
  }

  factory ChatColors.dark() {
    return const ChatColors(
      thinking: RoleColors(fill: Color(0xFF3F2E15), bar: Color(0xFFFBBF24)),
      answer: RoleColors(fill: Color(0xFF134E4A), bar: Color(0xFF2DD4BF)),
      stats: RoleColors(fill: Color(0xFF3F3F46), bar: Color(0xFFA1A1AA)),
      user: RoleColors(fill: Color(0xFF1E3A5F), bar: Color(0xFF60A5FA)),
    );
  }

  RoleColors forRole(ChatColorRole role) {
    return switch (role) {
      ChatColorRole.thinking => thinking,
      ChatColorRole.answer => answer,
      ChatColorRole.stats => stats,
      ChatColorRole.user => user,
    };
  }

  ChatColors withOverride(
    ChatColorRole role, {
    Color? fill,
    Color? bar,
  }) {
    final updated = forRole(role).copyWith(fill: fill, bar: bar);
    return switch (role) {
      ChatColorRole.thinking => copyWith(thinking: updated),
      ChatColorRole.answer => copyWith(answer: updated),
      ChatColorRole.stats => copyWith(stats: updated),
      ChatColorRole.user => copyWith(user: updated),
    };
  }

  @override
  ChatColors copyWith({
    RoleColors? thinking,
    RoleColors? answer,
    RoleColors? stats,
    RoleColors? user,
  }) {
    return ChatColors(
      thinking: thinking ?? this.thinking,
      answer: answer ?? this.answer,
      stats: stats ?? this.stats,
      user: user ?? this.user,
    );
  }

  @override
  ChatColors lerp(covariant ChatColors? other, double t) {
    if (other == null) {
      return this;
    }
    return ChatColors(
      thinking: RoleColors.lerp(thinking, other.thinking, t),
      answer: RoleColors.lerp(answer, other.answer, t),
      stats: RoleColors.lerp(stats, other.stats, t),
      user: RoleColors.lerp(user, other.user, t),
    );
  }
}
