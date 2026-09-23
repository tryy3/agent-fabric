import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:docking/docking.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

DockItemWidgets _stubs() => const DockItemWidgets(
  threads: SizedBox(),
  files: SizedBox(),
  chat: SizedBox(),
);

DockLayoutController _controller() {
  final c = DockLayoutController();
  addTearDown(c.dispose);
  return c;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

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
    final c = _controller();
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
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isFalse);
    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isTrue);
  });

  test('ensureCore focuses existing chat', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.threads;
    c.ensureCore(DockIds.chat);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.focusedItemId, DockIds.chat);
  });

  test('restoring files places them after threads and focuses files', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
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
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.threads);
    c.toggleCore(DockIds.files);
    c.toggleCore(DockIds.files);
    expect(_rowIds(c), [DockIds.files, DockIds.chat]);
  });

  test('threads insert leftmost and chat inserts rightmost', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.threads);
    c.toggleCore(DockIds.chat);
    c.toggleCore(DockIds.chat);
    c.toggleCore(DockIds.threads);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
  });

  test('ensureCore inserts a hidden chat on the right and focuses it', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.chat);
    c.focusedItemId = DockIds.threads;
    c.ensureCore(DockIds.chat);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.focusedItemId, DockIds.chat);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
  });

  test('ensureCore does not duplicate a visible core', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.ensureCore(DockIds.files);
    expect(_rowIds(c), [DockIds.threads, DockIds.files, DockIds.chat]);
    expect(c.focusedItemId, DockIds.files);
  });

  test('reopening files when threads is tabbed does not throw', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.layout.root = DockingRow([
      DockingTabs([
        DockingItem(
          id: DockIds.threads,
          name: DockIds.threads,
          widget: const SizedBox(),
        ),
        DockingItem(id: 'stub', name: 'stub', widget: const SizedBox()),
      ]),
      DockingItem(
        id: DockIds.files,
        name: DockIds.files,
        widget: const SizedBox(),
      ),
      DockingItem(
        id: DockIds.chat,
        name: DockIds.chat,
        widget: const SizedBox(),
      ),
    ]);

    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isFalse);
    expect(() => c.toggleCore(DockIds.files), returnsNormally);
    expect(c.hasItem(DockIds.files), isTrue);

    c.toggleCore(DockIds.files);
    expect(() => c.ensureCore(DockIds.files), returnsNormally);
    expect(c.hasItem(DockIds.files), isTrue);
    expect(c.hasItem(DockIds.threads), isTrue);
  });

  test('ensureCore reinserts a missing core when the root is a column', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
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
    final c = _controller();
    c.toggleCore(DockIds.files);
    c.ensureCore(DockIds.chat);
    expect(c.layout.root, isNull);
    expect(c.focusedItemId, isNull);
  });

  test('layout changes notify the controller', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    var count = 0;
    c.addListener(() => count++);
    c.layout.rebuild();
    expect(count, 1);
  });

  test('dispose stops forwarding layout notifications', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    final layout = c.layout;
    c.dispose();
    expect(layout.rebuild, returnsNormally);
  });

  test('dispose flushes a pending layout persist', () async {
    SharedPreferences.setMockInitialValues({});
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.dispose();

    String? saved;
    for (var i = 0; i < 20 && saved == null; i++) {
      await Future<void>.delayed(Duration.zero);
      final prefs = await SharedPreferences.getInstance();
      saved = prefs.getString(DockLayoutController.prefsKey);
    }
    expect(saved, isNotNull);
    expect(saved, contains('threads'));
    expect(saved, contains('chat'));
  });

  test('openDocument adds tab beside focused core', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.files;
    final view = OpenView(
      viewId: 'view-1',
      path: 'index.html',
      appId: WorkspaceAppId.textEditor,
    );
    c.openDocument(view: view, child: const Text('ed'));
    final id = DockIds.doc('index.html', WorkspaceAppId.textEditor);
    expect(c.hasItem(id), isTrue);
    final doc = c.layout.findDockingItem(id)!;
    expect(doc.parent, isA<DockingTabs>());
    final tabs = doc.parent! as DockingTabs;
    expect(
      [for (var i = 0; i < tabs.childrenCount; i++) tabs.childAt(i).id],
      [DockIds.files, id],
    );
    expect(tabs.childAt(tabs.selectedIndex).id, id);
  });

  test('openDocument toSide splits relative to focus', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.chat;
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    c.openDocument(
      view: OpenView(
        viewId: 'view-2',
        path: 'b.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('b'),
      toSide: true,
    );
    final aId = DockIds.doc('a.txt', WorkspaceAppId.textEditor);
    final bId = DockIds.doc('b.txt', WorkspaceAppId.textEditor);
    expect(c.hasItem(aId), isTrue);
    expect(c.hasItem(bId), isTrue);
    final aTabs = c.layout.findDockingTabsWithItem(aId);
    final b = c.layout.findDockingItem(bId)!;
    final split = b.parent;
    expect(aTabs, isNotNull);
    expect(split, anyOf(isA<DockingRow>(), isA<DockingColumn>()));
    expect(split!.contains(aTabs!), isTrue);
    expect(split.contains(b), isTrue);
  });

  test('clearDocuments removes only doc items', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    c.clearDocuments();
    expect(c.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)), isFalse);
    expect(c.hasItem(DockIds.chat), isTrue);
  });

  test('clearDocuments keeps a tab group weight after stripping docs', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.files;
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    final tabs = c.layout.findDockingTabsWithItem(DockIds.files)!;
    // ignore: invalid_use_of_internal_member
    tabs.updateWeight(0.4);
    c.clearDocuments();
    expect(c.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)), isFalse);
    expect(c.hasItem(DockIds.files), isTrue);
    final files = c.layout.findDockingItem(DockIds.files)!;
    expect(files.weight, closeTo(0.4, 0.001));
  });

  test('closeDocument removes that doc and keeps cores', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    final id = DockIds.doc('a.txt', WorkspaceAppId.textEditor);
    c.closeDocument(id);
    expect(c.hasItem(id), isFalse);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.hasItem(DockIds.files), isTrue);
  });

  test('openDocument focuses an existing doc without duplicating it', () {
    final c = _controller()..resetToDefault(widgets: _stubs());
    final view = OpenView(
      viewId: 'view-1',
      path: 'a.txt',
      appId: WorkspaceAppId.textEditor,
    );
    c.openDocument(view: view, child: const Text('a'));
    final id = DockIds.doc('a.txt', WorkspaceAppId.textEditor);
    final tabs = c.layout.findDockingTabsWithItem(id)!;
    tabs.selectedIndex = 0;
    c.focusedItemId = DockIds.files;
    c.openDocument(view: view, child: const Text('again'));
    expect(c.focusedItemId, id);
    expect(
      c.layout.layoutAreas().where((a) => a is DockingItem && a.id == id),
      hasLength(1),
    );
    expect(tabs.childAt(tabs.selectedIndex).id, id);
  });

  test('persist round-trip keeps cores and drops docs', () async {
    SharedPreferences.setMockInitialValues({});
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    await c.persist();

    final c2 = _controller();
    await c2.restore(widgets: _stubs());
    expect(c2.hasItem(DockIds.threads), isTrue);
    expect(c2.hasItem(DockIds.files), isTrue);
    expect(c2.hasItem(DockIds.chat), isTrue);
    expect(
      c2.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)),
      isFalse,
    );
  });

  test('persist round-trip keeps a closed core closed', () async {
    SharedPreferences.setMockInitialValues({});
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.files);
    await c.persist();

    final c2 = _controller();
    await c2.restore(widgets: _stubs());
    expect(c2.hasItem(DockIds.threads), isTrue);
    expect(c2.hasItem(DockIds.files), isFalse);
    expect(c2.hasItem(DockIds.chat), isTrue);
  });

  test('restore keeps a tab group weight after stripping docs', () async {
    SharedPreferences.setMockInitialValues({});
    final c = _controller()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.files;
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    final tabs = c.layout.findDockingTabsWithItem(DockIds.files)!;
    // ignore: invalid_use_of_internal_member
    tabs.updateWeight(0.4);
    await c.persist();

    final c2 = _controller();
    await c2.restore(widgets: _stubs());
    expect(
      c2.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)),
      isFalse,
    );
    final files = c2.layout.findDockingItem(DockIds.files)!;
    expect(files.weight, closeTo(0.4, 0.001));
  });

  test('corrupt prefs falls back to default', () async {
    SharedPreferences.setMockInitialValues({
      DockLayoutController.prefsKey: 'not-a-layout',
    });
    final c = _controller();
    await c.restore(widgets: _stubs());
    expect(c.hasItem(DockIds.chat), isTrue);
  });
}

List<dynamic> _rowIds(DockLayoutController c) {
  final row = c.layout.root! as DockingRow;
  return [
    for (var i = 0; i < row.childrenCount; i++)
      (row.childAt(i) as DockingItem).id,
  ];
}
