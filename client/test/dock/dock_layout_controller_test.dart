import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:docking/docking.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

DockItemWidgets _stubs() => const DockItemWidgets(
  threads: SizedBox(),
  files: SizedBox(),
  chat: SizedBox(),
);

void main() {
  test('DockIds names cores and docs', () {
    expect(DockIds.threads, 'threads');
    expect(DockIds.files, 'files');
    expect(DockIds.chat, 'chat');
    expect(
      DockIds.doc('/a/b.txt', WorkspaceAppId.textEditor),
      'doc:/a/b.txt:textEditor',
    );
    expect(DockIds.isDoc(DockIds.doc('x', WorkspaceAppId.webPreview)), isTrue);
    expect(DockIds.isDoc(DockIds.chat), isFalse);
    expect(DockIds.isDoc(1), isFalse);
    expect(DockIds.isDoc(null), isFalse);
  });

  test('default layout is threads | files | chat', () {
    final c = DockLayoutController();
    c.resetToDefault(widgets: _stubs());

    expect(c.hasItem(DockIds.threads), isTrue);
    expect(c.hasItem(DockIds.files), isTrue);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.layout.root, isA<DockingRow>());
    final row = c.layout.root! as DockingRow;
    // docking 1.16.2 exposes children via childAt, not a children getter.
    expect(
      [
        for (var i = 0; i < row.childrenCount; i++)
          (row.childAt(i) as DockingItem).id,
      ],
      [DockIds.threads, DockIds.files, DockIds.chat],
    );
    expect((row.childAt(0) as DockingItem).weight, 0.18);
    expect((row.childAt(1) as DockingItem).weight, 0.16);
    expect((row.childAt(2) as DockingItem).weight, 0.66);
    expect(c.focusedItemId, DockIds.chat);
  });
}
