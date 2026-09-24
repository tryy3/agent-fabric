import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import 'chat_colors.dart';

abstract final class AppTheme {
  static const seed = Color(0xFF0F766E);

  /// Bundled Latin UI font — covers punctuation without system / CDN Noto.
  static const fontFamily = 'NotoSans';

  /// Bundled mono for code / tool I/O (also registered as family `monospace`).
  static const monoFontFamily = 'NotoSansMono';

  static const _regularAsset = 'assets/fonts/NotoSans-Regular.ttf';
  static const _boldAsset = 'assets/fonts/NotoSans-Bold.ttf';
  static const _monoAsset = 'assets/fonts/NotoSansMono-Regular.ttf';

  /// Ensures asset fonts are registered before the first frame (Flutter web
  /// otherwise shapes with a partial Roboto / missing monospace and trips
  /// CDN Noto fallbacks).
  static Future<void> preloadFonts() async {
    final noto = FontLoader(fontFamily)
      ..addFont(rootBundle.load(_regularAsset))
      ..addFont(rootBundle.load(_boldAsset));
    final roboto = FontLoader('Roboto')
      ..addFont(rootBundle.load(_regularAsset))
      ..addFont(rootBundle.load(_boldAsset));
    final mono = FontLoader(monoFontFamily)
      ..addFont(rootBundle.load(_monoAsset));
    final monospaceAlias = FontLoader('monospace')
      ..addFont(rootBundle.load(_monoAsset));
    await Future.wait([
      noto.load(),
      roboto.load(),
      mono.load(),
      monospaceAlias.load(),
    ]);
  }

  static ThemeData light({ChatColors? chatColors}) {
    final scheme = ColorScheme.fromSeed(
      seedColor: seed,
      brightness: Brightness.light,
    );
    return _withFont(
      ThemeData(
        colorScheme: scheme,
        useMaterial3: true,
        fontFamily: fontFamily,
        extensions: [chatColors ?? ChatColors.light()],
      ),
    );
  }

  static ThemeData dark({ChatColors? chatColors}) {
    final scheme = ColorScheme.fromSeed(
      seedColor: seed,
      brightness: Brightness.dark,
    );
    return _withFont(
      ThemeData(
        colorScheme: scheme,
        useMaterial3: true,
        fontFamily: fontFamily,
        extensions: [chatColors ?? ChatColors.dark()],
      ),
    );
  }

  static ThemeData _withFont(ThemeData base) {
    return base.copyWith(
      textTheme: base.textTheme.apply(fontFamily: fontFamily),
      primaryTextTheme: base.primaryTextTheme.apply(fontFamily: fontFamily),
    );
  }
}
