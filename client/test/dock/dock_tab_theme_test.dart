import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:agent_fabric_client/dock/dock_tab_theme.dart';

void main() {
  test(
    'muted default text; selected tab is raised with a cyan top indicator',
    () {
      const scheme = ColorScheme.dark();
      final theme = buildDockTabTheme(scheme);
      expect(theme.tab.textStyle?.color, scheme.onSurfaceVariant);
      expect(theme.tab.selectedStatus.fontColor, scheme.onSurface);
      expect(theme.tabsArea.color, scheme.surface);
      expect(theme.tabsArea.normalButtonColor, scheme.onSurfaceVariant);
      final dec = theme.tab.selectedStatus.decoration;
      expect(dec?.color, scheme.surfaceContainerHigh);
      expect(dec?.border?.top.color, scheme.primary);
      expect(dec?.border?.top.width, 2);
      expect(dec?.border?.bottom, BorderSide.none);
    },
  );

  test('tabs and content areas form one rounded card on the canvas', () {
    const scheme = ColorScheme.dark();
    final theme = buildDockTabTheme(scheme);
    final top = theme.tabsArea.border;
    final bottom = theme.contentArea.decoration?.border;
    expect(top, isA<DockCardEdge>());
    expect(bottom, isA<DockCardEdge>());
    top as DockCardEdge;
    bottom as DockCardEdge;
    expect(top.isTop, isTrue);
    expect(bottom.isTop, isFalse);
    expect(top.canvasColor, scheme.surfaceContainerLowest);
    expect(top.bottom, BorderSide.none);
    expect(bottom.top, BorderSide.none);
  });
}
