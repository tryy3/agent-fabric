import 'package:flutter/material.dart';
import 'package:tabbed_view/tabbed_view.dart';

/// Hybrid+Soft dock tab chrome from the active [ColorScheme].
///
/// Selection is text contrast + fill merge into content — no primary top
/// accent. Lone panes otherwise each painted a loud selected dash. Inactive
/// titles stay muted through [TabThemeData.textStyle] (tabbed_view 1.18 has
/// no normal status theme). Menu divider stays thickness 4 /
/// [ColorScheme.outlineVariant], matching the shell split.
TabbedViewThemeData buildDockTabTheme(ColorScheme scheme) {
  final muted = scheme.onSurface.withValues(alpha: 0.45);
  final hover = scheme.onSurface.withValues(alpha: 0.72);
  final hairline = BorderSide(
    color: scheme.outlineVariant.withValues(alpha: 0.55),
    width: 1,
  );

  BoxDecoration tabChrome({Color? fill, bool trailingDivider = true}) {
    return BoxDecoration(
      color: fill,
      border: Border(
        right: trailingDivider ? hairline : BorderSide.none,
      ),
    );
  }

  return TabbedViewThemeData(
    tabsArea: TabsAreaThemeData(
      color: scheme.surfaceContainerHighest,
      middleGap: 0,
      gapBottomBorder: hairline,
      normalButtonColor: muted,
      hoverButtonColor: hover,
      disabledButtonColor: scheme.onSurface.withValues(alpha: 0.28),
    ),
    tab: TabThemeData(
      textStyle: TextStyle(fontSize: 13, color: muted),
      padding: const EdgeInsets.fromLTRB(10, 7, 6, 7),
      paddingWithoutButton: const EdgeInsets.fromLTRB(10, 7, 10, 7),
      decoration: tabChrome(),
      draggingDecoration: tabChrome(fill: scheme.surface),
      normalButtonColor: muted,
      hoverButtonColor: hover,
      disabledButtonColor: scheme.onSurface.withValues(alpha: 0.28),
      selectedStatus: TabStatusThemeData(
        fontColor: scheme.onSurface,
        // Same fill as content so a single selected tab does not float.
        decoration: tabChrome(fill: scheme.surface),
        normalButtonColor: scheme.onSurface.withValues(alpha: 0.7),
        hoverButtonColor: scheme.onSurface,
      ),
      highlightedStatus: TabStatusThemeData(
        fontColor: hover,
        decoration: tabChrome(
          fill: Color.alphaBlend(
            scheme.onSurface.withValues(alpha: 0.06),
            scheme.surfaceContainerHighest,
          ),
        ),
        normalButtonColor: hover,
        hoverButtonColor: hover,
      ),
    ),
    contentArea: ContentAreaThemeData(
      decoration: BoxDecoration(color: scheme.surface),
      decorationNoTabsArea: BoxDecoration(color: scheme.surface),
    ),
    menu: TabbedViewMenuThemeData(
      dividerThickness: 4,
      dividerColor: scheme.outlineVariant,
    ),
  );
}
