import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:agent_fabric_client/dock/dock_tab_theme.dart';

void main() {
  test(
    'muted default text; selected uses onSurface fill without primary accent',
    () {
      const scheme = ColorScheme.dark();
      final theme = buildDockTabTheme(scheme);
      // Normal tabs use tab.textStyle (no normalStatus in tabbed_view 1.18).
      expect(theme.tab.textStyle?.color?.a, lessThan(0.5));
      expect(theme.tab.selectedStatus.fontColor, scheme.onSurface);
      expect(theme.tabsArea.color, isNotNull);
      expect(
        theme.tabsArea.normalButtonColor,
        scheme.onSurface.withValues(alpha: 0.45),
      );
      expect(
        theme.tabsArea.hoverButtonColor,
        scheme.onSurface.withValues(alpha: 0.72),
      );
      final dec = theme.tab.selectedStatus.decoration;
      expect(dec?.color, scheme.surface);
      // No loud primary top dash on every lone selected pane.
      expect(dec?.border?.top.width ?? 0, 0);
    },
  );
}
