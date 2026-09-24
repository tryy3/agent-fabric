import 'dart:convert';

import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/shell/project_workspace_session.dart';
import 'package:agent_fabric_client/shell/workspace_document_ref.dart';
import 'package:agent_fabric_client/shell/workspace_memory.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:agent_fabric_client/workspace/workspace_controller.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _Catalog extends CatalogClient {
  _Catalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((request) async {
          throw StateError('use override methods: ${request.url}');
        }),
      );

  final Map<String, Map<String, Uint8List>> filesByProject = {};

  void seed(String projectId, Map<String, String> files) {
    filesByProject[projectId] = {
      for (final e in files.entries)
        e.key: Uint8List.fromList(utf8.encode(e.value)),
    };
  }

  @override
  Future<FsListing> listProjectFs(String projectId, {String path = '/'}) async {
    final files = filesByProject[projectId] ?? {};
    return FsListing(
      path: path,
      entries: [
        for (final name in files.keys)
          if (!name.contains('/'))
            FsEntry(name: name, isDir: false, size: files[name]!.length),
      ],
    );
  }

  @override
  Future<Uint8List> getProjectFile(String projectId, String path) async {
    final data = filesByProject[projectId]?[path];
    if (data == null) {
      throw CatalogException(statusCode: 404, message: 'not found');
    }
    return Uint8List.fromList(data);
  }
}

DockItemWidgets _items(WorkspaceController workspace) => DockItemWidgets(
  threads: const SizedBox(),
  files: Text('files-${workspace.hashCode}'),
  chat: const SizedBox(),
);

Future<void> coldRestore(
  ProjectWorkspaceSession session,
  WorkspaceMemory memory,
) async {
  final id = session.projectId;
  final expanded = await memory.expansion(id);
  final refs = await memory.documents(id);
  await session.workspace.setProjectId(
    id,
    restoreExpanded: expanded,
    notifyDocumentsCleared: false,
  );
  await session.workspace.restoreViews(refs, notifyDock: false);
  await session.dock.restore(
    widgets: session.itemWidgets,
    projectId: id,
    documentBuilder: (docId) {
      final parsed = DockIds.parseDoc(docId);
      if (parsed == null) {
        return const SizedBox.shrink();
      }
      final view = session.workspace.findView(parsed.path, parsed.appId);
      if (view == null) {
        return const SizedBox.shrink();
      }
      return Text(view.path);
    },
  );
  for (final view in session.workspace.openViews) {
    final docId = DockIds.doc(view.path, view.appId);
    if (!session.dock.hasItem(docId)) {
      session.dock.openDocument(view: view, child: Text(view.path));
    }
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('parked session swap keeps live docs and dirty buffer', () async {
    SharedPreferences.setMockInitialValues({});
    final catalog = _Catalog()
      ..seed('proj-a', {'a.txt': 'hello'})
      ..seed('proj-b', {'b.txt': 'other'});
    final store = ProjectSessionStore();
    addTearDown(store.disposeAll);

    final sessionA = createProjectSession(
      projectId: 'proj-a',
      catalog: catalog,
      itemWidgetsFor: _items,
    );
    await sessionA.workspace.setProjectId('proj-a');
    await sessionA.workspace.openWith('a.txt', WorkspaceAppId.textEditor);
    sessionA.workspace.documentFor('a.txt')!.replaceText('edited-a');
    sessionA.dock.openDocument(
      view: sessionA.workspace.openViews.single,
      child: const Text('a.txt'),
    );

    // Park A, open B.
    store.put(sessionA);
    final sessionB = createProjectSession(
      projectId: 'proj-b',
      catalog: catalog,
      itemWidgetsFor: _items,
    );
    await sessionB.workspace.setProjectId('proj-b');
    await sessionB.workspace.openWith('b.txt', WorkspaceAppId.textEditor);
    sessionB.dock.openDocument(
      view: sessionB.workspace.openViews.single,
      child: const Text('b.txt'),
    );

    // Switch back to A: remove from store — same live instance, no restore.
    final restored = store.remove('proj-a');
    expect(restored, same(sessionA));
    expect(sessionA.workspace.documentFor('a.txt')!.text, 'edited-a');
    expect(sessionA.workspace.documentFor('a.txt')!.isDirty, isTrue);
    expect(
      sessionA.dock.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)),
      isTrue,
    );

    store.put(sessionB);
    sessionA.dispose();
  });

  test('cold start restores docs from WorkspaceMemory', () async {
    SharedPreferences.setMockInitialValues({});
    final catalog = _Catalog()..seed('proj-a', {'a.txt': 'from-disk'});
    final memory = WorkspaceMemory();
    await memory.rememberDocuments('proj-a', const [
      WorkspaceDocumentRef(
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
        focused: true,
      ),
    ]);

    // Seed a saved layout that includes the doc tab.
    final seeder = createProjectSession(
      projectId: 'proj-a',
      catalog: catalog,
      itemWidgetsFor: _items,
    );
    await seeder.workspace.setProjectId('proj-a');
    await seeder.workspace.openWith('a.txt', WorkspaceAppId.textEditor);
    seeder.dock.openDocument(
      view: seeder.workspace.openViews.single,
      child: const Text('a.txt'),
    );
    await seeder.persist(memory);
    seeder.dispose();

    final fresh = createProjectSession(
      projectId: 'proj-a',
      catalog: catalog,
      itemWidgetsFor: _items,
    );
    addTearDown(fresh.dispose);
    await coldRestore(fresh, memory);

    expect(fresh.workspace.openViews.single.path, 'a.txt');
    expect(fresh.workspace.documentFor('a.txt')!.text, 'from-disk');
    expect(fresh.workspace.documentFor('a.txt')!.isDirty, isFalse);
    expect(
      fresh.dock.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)),
      isTrue,
    );
  });

  test('restoreViews still applies dirty text on cold path', () async {
    final catalog = _Catalog()..seed('proj-a', {'a.txt': 'disk'});
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj-a');
    await workspace.restoreViews(
      const [
        WorkspaceDocumentRef(
          path: 'a.txt',
          appId: WorkspaceAppId.textEditor,
          viewMode: EditorViewMode.preview,
          focused: true,
        ),
      ],
      dirtyTextByPath: const {'a.txt': 'unsaved'},
      notifyDock: false,
    );

    expect(workspace.documentFor('a.txt')!.text, 'unsaved');
    expect(workspace.documentFor('a.txt')!.isDirty, isTrue);
    expect(
      workspace.viewModeFor(workspace.openViews.single.viewId),
      EditorViewMode.preview,
    );
  });
}
