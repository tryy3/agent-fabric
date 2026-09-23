import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:agent_fabric_client/dock/dock_tab_theme.dart';

void main() {
  test(
    'muted default text; selected uses onSurface and primary top border',
    () {
      const scheme = ColorScheme.dark();
      final theme = buildDockTabTheme(scheme);
      // Normal tabs use tab.textStyle (no normalStatus in tabbed_view 1.18).
      expect(theme.tab.textStyle?.color?.a, lessThan(0.5));
      expect(theme.tab.selectedStatus.fontColor, scheme.onSurface);
      expect(theme.tabsArea.color, isNotNull);
      final dec = theme.tab.selectedStatus.decoration;
      expect(dec?.border?.top.color, scheme.primary);
      expect(dec?.border?.top.width, greaterThanOrEqualTo(1.5));
    },
  );
}
