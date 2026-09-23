import 'package:flutter/material.dart';
import 'package:tabbed_view/tabbed_view.dart';

/// Hybrid+Soft dock tab chrome from the active [ColorScheme].
///
/// Inactive titles stay muted through [TabThemeData.textStyle] because
/// tabbed_view 1.18 has no normal status theme. The menu divider stays
/// thickness 4 and [ColorScheme.outlineVariant], matching the shell split.
TabbedViewThemeData buildDockTabTheme(ColorScheme scheme) {
  final muted = scheme.onSurface.withValues(alpha: 0.45);
  final hover = scheme.onSurface.withValues(alpha: 0.72);
  final divider = BorderSide(color: scheme.outlineVariant, width: 1);

  BoxDecoration tabChrome({Color? fill, BorderSide? top}) {
    return BoxDecoration(
      color: fill,
      border: Border(top: top ?? BorderSide.none, right: divider),
    );
  }

  return TabbedViewThemeData(
    tabsArea: TabsAreaThemeData(
      color: scheme.surfaceContainerHighest,
      middleGap: 0,
      gapBottomBorder: divider,
      normalButtonColor: muted,
      hoverButtonColor: hover,
      disabledButtonColor: scheme.onSurface.withValues(alpha: 0.28),
    ),
    tab: TabThemeData(
      textStyle: TextStyle(fontSize: 13, color: muted),
      padding: const EdgeInsets.fromLTRB(8, 6, 4, 6),
      paddingWithoutButton: const EdgeInsets.fromLTRB(8, 6, 8, 6),
      decoration: tabChrome(),
      draggingDecoration: tabChrome(fill: scheme.surface),
      normalButtonColor: muted,
      hoverButtonColor: hover,
      disabledButtonColor: scheme.onSurface.withValues(alpha: 0.28),
      selectedStatus: TabStatusThemeData(
        fontColor: scheme.onSurface,
        decoration: tabChrome(
          fill: scheme.surface,
          top: BorderSide(color: scheme.primary, width: 2),
        ),
        normalButtonColor: scheme.onSurface,
        hoverButtonColor: scheme.onSurface,
      ),
      highlightedStatus: TabStatusThemeData(
        fontColor: hover,
        decoration: tabChrome(
          fill: Color.alphaBlend(
            scheme.onSurface.withValues(alpha: 0.08),
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
