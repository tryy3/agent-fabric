import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import 'chat_colors.dart';
import 'design_tokens.dart';

abstract final class AppTheme {
  static const seed = Color(0xFF22D3D1);

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
    return _theme(
      DesignTokens.light(),
      Brightness.light,
      chatColors ?? ChatColors.light(),
    );
  }

  static ThemeData dark({ChatColors? chatColors}) {
    return _theme(
      DesignTokens.dark(),
      Brightness.dark,
      chatColors ?? ChatColors.dark(),
    );
  }

  static ThemeData _theme(
    DesignTokens tokens,
    Brightness brightness,
    ChatColors chatColors,
  ) {
    final scheme = ColorScheme(
      brightness: brightness,
      primary: tokens.primary,
      onPrimary: tokens.background,
      primaryContainer: tokens.primaryMuted,
      onPrimaryContainer: tokens.textPrimary,
      secondary: tokens.secondary,
      onSecondary: tokens.background,
      secondaryContainer: tokens.surfaceActive,
      onSecondaryContainer: tokens.textPrimary,
      tertiary: tokens.success,
      onTertiary: tokens.background,
      error: tokens.error,
      onError: tokens.textPrimary,
      surface: tokens.surface,
      onSurface: tokens.textPrimary,
      onSurfaceVariant: tokens.textSecondary,
      outline: tokens.borderStrong,
      outlineVariant: tokens.border,
      shadow: const Color(0xFF000000),
      surfaceTint: tokens.primary,
      surfaceContainerLowest: tokens.background,
      surfaceContainerLow: tokens.sidebar,
      surfaceContainer: tokens.surface,
      surfaceContainerHigh: tokens.surfaceRaised,
      surfaceContainerHighest: tokens.surfaceRaised,
      inverseSurface: tokens.textPrimary,
      onInverseSurface: tokens.background,
    );
    final text = _textTheme(tokens);
    return ThemeData(
      colorScheme: scheme,
      useMaterial3: true,
      fontFamily: fontFamily,
      scaffoldBackgroundColor: tokens.background,
      canvasColor: tokens.background,
      dividerColor: tokens.border,
      textTheme: text,
      primaryTextTheme: text,
      extensions: [chatColors, tokens],
      iconTheme: IconThemeData(color: tokens.textSecondary, size: 18),
      focusColor: tokens.focus,
    );
  }

  static TextTheme _textTheme(DesignTokens tokens) {
    return TextTheme(
      headlineLarge: tokens.headingLg(),
      headlineMedium: tokens.headingMd(),
      titleMedium: tokens.labelMd().copyWith(fontSize: 14),
      bodyLarge: tokens.bodyMd().copyWith(fontSize: 15),
      bodyMedium: tokens.bodyMd(),
      bodySmall: tokens.bodySm().copyWith(color: tokens.textSecondary),
      labelLarge: tokens.labelMd(),
      labelMedium: tokens.labelSm().copyWith(color: tokens.textSecondary),
      labelSmall: tokens.caption().copyWith(color: tokens.textMuted),
    );
  }
}
