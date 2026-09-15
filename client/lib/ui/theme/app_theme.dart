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
