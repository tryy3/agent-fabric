import 'package:material_ui/material_ui.dart';

import 'chat_colors.dart';

abstract final class AppTheme {
  static const seed = Color(0xFF0F766E);

  /// Bundled Latin UI font — covers ellipsis / middle-dot without system Noto.
  static const fontFamily = 'NotoSans';

  static ThemeData light({ChatColors? chatColors}) {
    final scheme = ColorScheme.fromSeed(
      seedColor: seed,
      brightness: Brightness.light,
    );
    return ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      fontFamily: fontFamily,
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
      fontFamily: fontFamily,
      extensions: [chatColors ?? ChatColors.dark()],
    );
  }
}
