import 'package:flutter/material.dart';
import 'package:tabbed_view/tabbed_view.dart';

import '../ui/theme/app_theme.dart';

/// Corner radius of each dock pane card.
const double dockCardRadius = 7;

/// Dock tab chrome from the workbench [ColorScheme].
///
/// Each tab group renders as a rounded card on the canvas
/// ([ColorScheme.surfaceContainerLowest]). Inactive titles stay muted through
/// [TabThemeData.textStyle] (tabbed_view 1.18 has no normal status theme). The
/// selected tab is raised with a cyan top indicator.
TabbedViewThemeData buildDockTabTheme(ColorScheme scheme) {
  final muted = scheme.onSurfaceVariant;
  final hover = scheme.onSurface.withValues(alpha: 0.86);
  final hairline = BorderSide(color: scheme.outlineVariant, width: 1);
  final canvas = scheme.surfaceContainerLowest;

  BoxDecoration tabChrome({Color? fill, bool selected = false}) {
    return BoxDecoration(
      color: fill,
      border: Border(
        top: BorderSide(
          color: selected ? scheme.primary : Colors.transparent,
          width: 2,
        ),
        bottom: selected ? BorderSide.none : hairline,
      ),
    );
  }

  return TabbedViewThemeData(
    tabsArea: TabsAreaThemeData(
      color: scheme.surface,
      border: DockCardEdge(
        side: hairline,
        canvasColor: canvas,
        radius: dockCardRadius,
        isTop: true,
      ),
      initialGap: 6,
      middleGap: 2,
      gapBottomBorder: hairline,
      equalHeights: EqualHeights.all,
      normalButtonColor: muted,
      hoverButtonColor: hover,
      disabledButtonColor: scheme.onSurface.withValues(alpha: 0.28),
    ),
    tab: TabThemeData(
      textStyle: TextStyle(
        fontFamily: AppTheme.fontFamily,
        fontSize: 13,
        fontWeight: FontWeight.w500,
        color: muted,
      ),
      padding: const EdgeInsets.fromLTRB(12, 8, 8, 10),
      paddingWithoutButton: const EdgeInsets.fromLTRB(12, 8, 12, 10),
      buttonsOffset: 8,
      buttonPadding: const EdgeInsets.all(2),
      buttonIconSize: 16,
      decoration: tabChrome(),
      draggingDecoration: tabChrome(fill: scheme.surfaceContainerHigh),
      normalButtonColor: muted,
      hoverButtonColor: hover,
      disabledButtonColor: scheme.onSurface.withValues(alpha: 0.28),
      selectedStatus: TabStatusThemeData(
        fontColor: scheme.onSurface,
        decoration: tabChrome(
          fill: scheme.surfaceContainerHigh,
          selected: true,
        ),
        normalButtonColor: scheme.onSurface.withValues(alpha: 0.7),
        hoverButtonColor: scheme.onSurface,
      ),
      highlightedStatus: TabStatusThemeData(
        fontColor: hover,
        decoration: tabChrome(
          fill: Color.alphaBlend(
            scheme.onSurface.withValues(alpha: 0.04),
            scheme.surface,
          ),
        ),
        normalButtonColor: hover,
        hoverButtonColor: hover,
      ),
    ),
    contentArea: ContentAreaThemeData(
      decoration: BoxDecoration(
        color: scheme.surface,
        border: DockCardEdge(
          side: hairline,
          canvasColor: canvas,
          radius: dockCardRadius,
          isTop: false,
        ),
      ),
      decorationNoTabsArea: BoxDecoration(
        color: scheme.surface,
        borderRadius: BorderRadius.circular(dockCardRadius),
        border: Border.fromBorderSide(hairline),
      ),
      padding: const EdgeInsets.only(top: 4),
    ),
    menu: TabbedViewMenuThemeData(
      dividerThickness: 4,
      dividerColor: scheme.outlineVariant,
    ),
  );
}

/// Outline for the top (tabs) or bottom (content) half of a dock card.
///
/// tabbed_view only paints square decorations for its tabs area, so the
/// rounded outer corners are masked with [canvasColor] here. The join between
/// the two halves stays open.
class DockCardEdge extends Border {
  const DockCardEdge({
    required BorderSide side,
    required this.canvasColor,
    required this.radius,
    required this.isTop,
  }) : super(
         top: isTop ? side : BorderSide.none,
         bottom: isTop ? BorderSide.none : side,
         left: side,
         right: side,
       );

  final Color canvasColor;
  final double radius;
  final bool isTop;

  @override
  void paint(
    Canvas canvas,
    Rect rect, {
    TextDirection? textDirection,
    BoxShape shape = BoxShape.rectangle,
    BorderRadius? borderRadius,
  }) {
    final side = left;
    // Extend past the join so only the outer corners are rounded.
    final reach = radius + side.width;
    final card = RRect.fromRectAndRadius(
      isTop
          ? Rect.fromLTRB(rect.left, rect.top, rect.right, rect.bottom + reach)
          : Rect.fromLTRB(rect.left, rect.top - reach, rect.right, rect.bottom),
      Radius.circular(radius),
    );
    canvas.save();
    canvas.clipRect(rect);
    canvas.drawPath(
      Path()
        ..fillType = PathFillType.evenOdd
        ..addRect(rect)
        ..addRRect(card),
      Paint()..color = canvasColor,
    );
    if (side.style != BorderStyle.none && side.width > 0) {
      canvas.drawRRect(
        card.deflate(side.width / 2),
        Paint()
          ..color = side.color
          ..style = PaintingStyle.stroke
          ..strokeWidth = side.width,
      );
    }
    canvas.restore();
  }

  @override
  bool operator ==(Object other) =>
      other is DockCardEdge &&
      other.left == left &&
      other.canvasColor == canvasColor &&
      other.radius == radius &&
      other.isTop == isTop;

  @override
  int get hashCode => Object.hash(left, canvasColor, radius, isTop);
}

/// Clips pane content to the bottom corners of its dock card.
class DockCardBody extends StatelessWidget {
  const DockCardBody({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: const BorderRadius.vertical(
        bottom: Radius.circular(dockCardRadius - 1),
      ),
      child: child,
    );
  }
}
