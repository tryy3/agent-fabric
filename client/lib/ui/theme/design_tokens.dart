import 'package:material_ui/material_ui.dart';

import 'app_theme.dart';

/// Visual tokens from DESIGN.md.
///
/// Dark values match the spec. Light values are a derived workbench so
/// [ThemeMode] still switches without a second product palette.
class DesignTokens extends ThemeExtension<DesignTokens> {
  const DesignTokens({
    required this.background,
    required this.sidebar,
    required this.surface,
    required this.surfaceRaised,
    required this.surfaceActive,
    required this.border,
    required this.borderStrong,
    required this.textPrimary,
    required this.textSecondary,
    required this.textMuted,
    required this.primary,
    required this.primaryHover,
    required this.primaryMuted,
    required this.secondary,
    required this.success,
    required this.warning,
    required this.error,
    required this.focus,
  });

  final Color background;
  final Color sidebar;
  final Color surface;
  final Color surfaceRaised;
  final Color surfaceActive;
  final Color border;
  final Color borderStrong;
  final Color textPrimary;
  final Color textSecondary;
  final Color textMuted;
  final Color primary;
  final Color primaryHover;
  final Color primaryMuted;
  final Color secondary;
  final Color success;
  final Color warning;
  final Color error;
  final Color focus;

  static const double sidebarWidth = 304;
  static const double toolbarHeight = 56;
  static const double tabHeight = 40;
  static const double rowHeight = 36;
  static const double treeRowHeight = 30;
  static const double panelMinWidth = 240;
  static const double chatMinWidth = 280;
  static const double treeIndent = 16;

  static const double radiusXs = 3;
  static const double radiusSm = 5;
  static const double radiusMd = 7;
  static const double radiusLg = 10;

  factory DesignTokens.dark() {
    return const DesignTokens(
      background: Color(0xFF071017),
      sidebar: Color(0xFF08131A),
      surface: Color(0xFF0B161E),
      surfaceRaised: Color(0xFF101D26),
      surfaceActive: Color(0xFF12303B),
      border: Color(0xFF20303B),
      borderStrong: Color(0xFF304452),
      textPrimary: Color(0xFFF0F5F7),
      textSecondary: Color(0xFFB7C3CB),
      textMuted: Color(0xFF7F929F),
      primary: Color(0xFF22D3D1),
      primaryHover: Color(0xFF4DE0DE),
      primaryMuted: Color(0xFF123D43),
      secondary: Color(0xFF38A5FF),
      success: Color(0xFF35C97A),
      warning: Color(0xFFF59E42),
      error: Color(0xFFF06464),
      focus: Color(0xFF55D9E2),
    );
  }

  /// Lighter canvas that keeps the same cyan interaction color readable.
  factory DesignTokens.light() {
    return const DesignTokens(
      background: Color(0xFFF3F6F8),
      sidebar: Color(0xFFE7EEF2),
      surface: Color(0xFFFBFCFD),
      surfaceRaised: Color(0xFFFFFFFF),
      surfaceActive: Color(0xFFD7F3F2),
      border: Color(0xFFD5DEE4),
      borderStrong: Color(0xFFB7C6D0),
      textPrimary: Color(0xFF102027),
      textSecondary: Color(0xFF3E515C),
      textMuted: Color(0xFF6A7C87),
      primary: Color(0xFF0E8A88),
      primaryHover: Color(0xFF14A8A5),
      primaryMuted: Color(0xFFD3F4F3),
      secondary: Color(0xFF1D6FBF),
      success: Color(0xFF1F8A52),
      warning: Color(0xFFB86A12),
      error: Color(0xFFC43B3B),
      focus: Color(0xFF0E8A88),
    );
  }

  TextStyle headingLg() => _style(24, FontWeight.w600, 1.25, -0.36);

  TextStyle headingMd() => _style(18, FontWeight.w600, 1.3, -0.18);

  TextStyle bodyMd() => _style(14, FontWeight.w400, 1.5, 0);

  TextStyle bodySm() => _style(13, FontWeight.w400, 1.45, 0);

  TextStyle labelMd() => _style(13, FontWeight.w500, 1.25, 0);

  TextStyle labelSm() => _style(12, FontWeight.w500, 1.25, 0);

  TextStyle caption() => _style(11, FontWeight.w500, 1.35, 0);

  TextStyle code() => TextStyle(
    fontFamily: AppTheme.monoFontFamily,
    fontSize: 13,
    fontWeight: FontWeight.w400,
    height: 1.55,
    color: textPrimary,
  );

  TextStyle _style(
    double size,
    FontWeight weight,
    double height,
    double spacing,
  ) {
    return TextStyle(
      fontFamily: AppTheme.fontFamily,
      fontSize: size,
      fontWeight: weight,
      height: height,
      letterSpacing: spacing,
      color: textPrimary,
    );
  }

  @override
  DesignTokens copyWith({
    Color? background,
    Color? sidebar,
    Color? surface,
    Color? surfaceRaised,
    Color? surfaceActive,
    Color? border,
    Color? borderStrong,
    Color? textPrimary,
    Color? textSecondary,
    Color? textMuted,
    Color? primary,
    Color? primaryHover,
    Color? primaryMuted,
    Color? secondary,
    Color? success,
    Color? warning,
    Color? error,
    Color? focus,
  }) {
    return DesignTokens(
      background: background ?? this.background,
      sidebar: sidebar ?? this.sidebar,
      surface: surface ?? this.surface,
      surfaceRaised: surfaceRaised ?? this.surfaceRaised,
      surfaceActive: surfaceActive ?? this.surfaceActive,
      border: border ?? this.border,
      borderStrong: borderStrong ?? this.borderStrong,
      textPrimary: textPrimary ?? this.textPrimary,
      textSecondary: textSecondary ?? this.textSecondary,
      textMuted: textMuted ?? this.textMuted,
      primary: primary ?? this.primary,
      primaryHover: primaryHover ?? this.primaryHover,
      primaryMuted: primaryMuted ?? this.primaryMuted,
      secondary: secondary ?? this.secondary,
      success: success ?? this.success,
      warning: warning ?? this.warning,
      error: error ?? this.error,
      focus: focus ?? this.focus,
    );
  }

  @override
  DesignTokens lerp(covariant DesignTokens? other, double t) {
    if (other == null) {
      return this;
    }
    Color mix(Color a, Color b) => Color.lerp(a, b, t)!;
    return DesignTokens(
      background: mix(background, other.background),
      sidebar: mix(sidebar, other.sidebar),
      surface: mix(surface, other.surface),
      surfaceRaised: mix(surfaceRaised, other.surfaceRaised),
      surfaceActive: mix(surfaceActive, other.surfaceActive),
      border: mix(border, other.border),
      borderStrong: mix(borderStrong, other.borderStrong),
      textPrimary: mix(textPrimary, other.textPrimary),
      textSecondary: mix(textSecondary, other.textSecondary),
      textMuted: mix(textMuted, other.textMuted),
      primary: mix(primary, other.primary),
      primaryHover: mix(primaryHover, other.primaryHover),
      primaryMuted: mix(primaryMuted, other.primaryMuted),
      secondary: mix(secondary, other.secondary),
      success: mix(success, other.success),
      warning: mix(warning, other.warning),
      error: mix(error, other.error),
      focus: mix(focus, other.focus),
    );
  }
}

DesignTokens designTokensOf(BuildContext context) {
  return Theme.of(context).extension<DesignTokens>() ?? DesignTokens.dark();
}
