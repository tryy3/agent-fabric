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

  test('toggleCore removes and restores files', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isFalse);
    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isTrue);
  });

  test('ensureCore focuses existing chat', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.threads;
    c.ensureCore(DockIds.chat);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.focusedItemId, DockIds.chat);
  });

  test('restoring files places them after threads and focuses files', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.files);
    c.focusedItemId = DockIds.threads;
    c.toggleCore(DockIds.files);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
    expect(c.focusedItemId, DockIds.files);
    expect((c.layout.root! as DockingRow).childAt(1), isA<DockingItem>());
    expect(
      ((c.layout.root! as DockingRow).childAt(1) as DockingItem).weight,
      0.16,
    );
  });

  test('files insert leftmost when threads are hidden', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.threads);
    c.toggleCore(DockIds.files);
    c.toggleCore(DockIds.files);
    expect(_rowIds(c), [DockIds.files, DockIds.chat]);
  });

  test('threads insert leftmost and chat inserts rightmost', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.threads);
    c.toggleCore(DockIds.chat);
    c.toggleCore(DockIds.chat);
    c.toggleCore(DockIds.threads);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
  });

  test('ensureCore inserts a hidden chat on the right and focuses it', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.chat);
    c.focusedItemId = DockIds.threads;
    c.ensureCore(DockIds.chat);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.focusedItemId, DockIds.chat);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
  });

  test('ensureCore does not duplicate a visible core', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.ensureCore(DockIds.files);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
    expect(c.focusedItemId, DockIds.files);
  });

  test('ensureCore reinserts a missing core when the root is a column', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.layout.root = DockingColumn([
      DockingItem(
        id: DockIds.threads,
        name: DockIds.threads,
        widget: const SizedBox(),
      ),
      DockingItem(
        id: DockIds.files,
        name: DockIds.files,
        widget: const SizedBox(),
      ),
    ]);

    expect(() => c.ensureCore(DockIds.chat), returnsNormally);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.focusedItemId, DockIds.chat);
    expect(c.hasItem(DockIds.threads), isTrue);
    expect(c.hasItem(DockIds.files), isTrue);
  });

  test('toggle and ensure are no-ops before widgets are provided', () {
    final c = DockLayoutController();
    c.toggleCore(DockIds.files);
    c.ensureCore(DockIds.chat);
    expect(c.layout.root, isNull);
    expect(c.focusedItemId, isNull);
  });

  test('layout changes notify the controller', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    var count = 0;
    c.addListener(() => count++);
    c.layout.rebuild();
    expect(count, 1);
  });

  test('dispose stops forwarding layout notifications', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    final layout = c.layout;
    c.dispose();
    expect(layout.rebuild, returnsNormally);
  });
}

List<dynamic> _rowIds(DockLayoutController c) {
  final row = c.layout.root! as DockingRow;
  return [
    for (var i = 0; i < row.childrenCount; i++)
      (row.childAt(i) as DockingItem).id,
  ];
}
