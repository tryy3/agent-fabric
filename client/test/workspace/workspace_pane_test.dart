import 'dart:convert';
import 'dart:typed_data';

import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/dock/dock_view_body.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:agent_fabric_client/workspace/workspace_controller.dart';
import 'package:agent_fabric_client/workspace/workspace_pane.dart';
import 'package:docking/docking.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class MemoryWorkspaceCatalog extends CatalogClient {
  MemoryWorkspaceCatalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((request) async {
          throw StateError('use override methods: ${request.url}');
        }),
      );

  final Map<String, Uint8List> files = {};
  final Set<String> dirs = {'.'};
  int listCalls = 0;
  int putCalls = 0;
  int checkpointCalls = 0;
  int restoreCalls = 0;
  final List<GitCommit> commits = [];
  String? lastCheckpointLabel;
  String? lastRestoreSha;
  String lastDiff =
      '--- a/index.html\n+++ b/index.html\n-<h1>old</h1>\n+<h1>new</h1>\n';

  @override
  Future<FsListing> listProjectFs(String projectId, {String path = '/'}) async {
    listCalls++;
    final dir = path == '/' ? '.' : path;
    final entries = <FsEntry>[];
    final seen = <String>{};
    for (final d in dirs) {
      if (d == '.' || d == dir) {
        continue;
      }
      final parent = d.contains('/') ? d.substring(0, d.lastIndexOf('/')) : '.';
      if (parent == dir) {
        final name = d.split('/').last;
        if (seen.add(name)) {
          entries.add(FsEntry(name: name, isDir: true));
        }
      }
    }
    for (final file in files.keys) {
      final parent = file.contains('/')
          ? file.substring(0, file.lastIndexOf('/'))
          : '.';
      if (parent == dir || (dir == '.' && !file.contains('/'))) {
        final name = file.split('/').last;
        if (seen.add(name)) {
          entries.add(
            FsEntry(name: name, isDir: false, size: files[file]!.length),
          );
        }
      }
    }
    return FsListing(path: path, entries: entries);
  }

  @override
  Future<Uint8List> getProjectFile(String projectId, String path) async {
    final data = files[path];
    if (data == null) {
      throw CatalogException(statusCode: 404, message: 'not found');
    }
    return Uint8List.fromList(data);
  }

  @override
  Future<void> putProjectFile(
    String projectId,
    String path,
    List<int> bytes, {
    String contentType = 'application/octet-stream',
  }) async {
    putCalls++;
    files[path] = Uint8List.fromList(bytes);
    if (path.contains('/')) {
      dirs.add(path.substring(0, path.lastIndexOf('/')));
    }
  }

  @override
  Future<void> deleteProjectFile(String projectId, String path) async {
    files.remove(path);
    dirs.remove(path);
  }

  @override
  Future<void> createProjectDir(String projectId, String path) async {
    dirs.add(path);
  }

  @override
  Uri previewUri(String projectId, String path) {
    return Uri.parse(
      'http://catalog.test/v1/projects/$projectId/preview/$path',
    );
  }

  @override
  Future<List<GitCommit>> listProjectCommits(String projectId) async {
    return List<GitCommit>.from(commits);
  }

  @override
  Future<Checkpoint> createCheckpoint(
    String projectId, {
    required String label,
    String? threadId,
  }) async {
    checkpointCalls++;
    lastCheckpointLabel = label;
    final created = Checkpoint(
      id: 'chk_$checkpointCalls',
      projectId: projectId,
      sha: 'abc1234',
      label: label,
      createdAt: DateTime.utc(2026, 9, 20),
    );
    commits.insert(
      0,
      GitCommit(
        sha: created.sha,
        message: 'checkpoint: $label',
        checkpointId: created.id,
        label: label,
        committedAt: created.createdAt,
      ),
    );
    return created;
  }

  @override
  Future<void> restoreProject(String projectId, {required String sha}) async {
    restoreCalls++;
    lastRestoreSha = sha;
  }

  @override
  Future<DiffResult> projectDiff(
    String projectId, {
    required String from,
    String to = '',
  }) async {
    return DiffResult(from: from, to: to, diff: lastDiff);
  }
}

void main() {
  test('listProjectFs GET /v1/projects/{id}/fs', () async {
    final client = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/projects/proj_1/fs');
        expect(request.url.queryParameters['path'], '/');
        return http.Response(
          jsonEncode({
            'path': '/',
            'entries': [
              {
                'name': 'index.html',
                'isDir': false,
                'size': 12,
                'modTime': '2026-09-20T00:00:00Z',
              },
            ],
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final listing = await client.listProjectFs('proj_1');
    expect(listing.entries, hasLength(1));
    expect(listing.entries.single.name, 'index.html');
    expect(listing.entries.single.isDir, isFalse);
  });

  test('putProjectFile PUT raw bytes', () async {
    final client = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient((request) async {
        expect(request.method, 'PUT');
        expect(request.url.path, '/v1/projects/proj_1/files');
        expect(request.url.queryParameters['path'], 'index.html');
        expect(request.bodyBytes, utf8.encode('<h1>hi</h1>'));
        return http.Response('', 204);
      }),
    );
    await client.putProjectFile(
      'proj_1',
      'index.html',
      utf8.encode('<h1>hi</h1>'),
    );
  });

  test('previewUri is catalog preview path not ACP', () {
    final client = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient((_) async => http.Response('', 404)),
    );
    expect(
      client.previewUri('proj_1', 'index.html').toString(),
      'http://catalog.test/v1/projects/proj_1/preview/index.html',
    );
  });

  test('html default app is editor and web preview is associated', () {
    final assoc = associationFor('index.html');
    expect(assoc.defaultApp, WorkspaceAppId.textEditor);
    expect(assoc.apps, contains(WorkspaceAppId.webPreview));
  });

  test('WorkspaceController save writes catalog bytes', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>old</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');
    expect(workspace.openViews, hasLength(1));
    expect(workspace.openViews.single.path, 'index.html');
    final doc = workspace.documentFor('index.html')!;
    doc.replaceText('<h1>new</h1>');
    expect(doc.isDirty, isTrue);
    await workspace.savePath('index.html');
    expect(doc.isDirty, isFalse);
    expect(utf8.decode(catalog.files['index.html']!), '<h1>new</h1>');
    expect(catalog.putCalls, 1);
  });

  test('opening html web preview notifies toSide when editor open', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    final opened = <({OpenView view, bool toSide})>[];
    workspace.onViewOpened = (view, {required bool toSide}) {
      opened.add((view: view, toSide: toSide));
    };
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');
    await workspace.openWith('index.html', WorkspaceAppId.webPreview);
    expect(workspace.openViews, hasLength(2));
    expect(workspace.openViews[0].path, 'index.html');
    expect(workspace.openViews[0].appId, WorkspaceAppId.textEditor);
    expect(workspace.openViews[1].appId, WorkspaceAppId.webPreview);
    expect(workspace.focusedView?.appId, WorkspaceAppId.webPreview);
    expect(opened, hasLength(2));
    expect(opened[0].toSide, isFalse);
    expect(opened[1].view.appId, WorkspaceAppId.webPreview);
    expect(opened[1].toSide, isTrue);
  });

  test('second openDefault keeps one view and notifies again', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    final opened = <({OpenView view, bool toSide})>[];
    workspace.onViewOpened = (view, {required bool toSide}) {
      opened.add((view: view, toSide: toSide));
    };
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');
    await workspace.openDefault('index.html');
    expect(workspace.openViews, hasLength(1));
    expect(opened, hasLength(2));
    expect(opened[1].view.viewId, opened[0].view.viewId);
    expect(opened[1].toSide, isFalse);
    expect(workspace.focusedViewId, opened[0].view.viewId);
  });

  test('closeView drops the view and clears its document', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    final closed = <OpenView>[];
    workspace.onViewClosed = closed.add;
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');
    final viewId = workspace.openViews.single.viewId;
    await workspace.closeView(viewId);
    expect(closed, hasLength(1));
    expect(closed.single.viewId, viewId);
    expect(workspace.openViews, isEmpty);
    expect(workspace.focusedView, isNull);
    expect(workspace.documentFor('index.html'), isNull);
  });

  test('setProjectId clears open views and refreshes the tree', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    var cleared = 0;
    workspace.onDocumentsCleared = () => cleared++;
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');
    expect(workspace.children['.'], isNotEmpty);
    expect(cleared, 1);
    await workspace.setProjectId('proj_2');
    expect(workspace.openViews, isEmpty);
    expect(workspace.focusedView, isNull);
    expect(cleared, 2);
  });

  test('agent refresh reloads tree and dirty docs get diskChanged', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('one'));
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');
    workspace.documentFor('index.html')!.replaceText('local');
    catalog.files['index.html'] = Uint8List.fromList(utf8.encode('agent'));
    catalog.files['app.js'] = Uint8List.fromList(utf8.encode('js'));
    await workspace.refreshAfterAgentTurn();
    expect(
      workspace.children['.']!.map((e) => e.name),
      containsAll(['index.html', 'app.js']),
    );
    expect(workspace.documentFor('index.html')!.diskChanged, isTrue);
    expect(workspace.documentFor('index.html')!.text, 'local');
  });

  testWidgets('explorer lists files and save button puts content', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1200, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: WorkspacePane(controller: workspace)),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('index.html'), findsWidgets);
    await tester.tap(find.byKey(const Key('file-row-index.html')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('save-file')), findsOneWidget);
    workspace.documentFor('index.html')!.replaceText('<h1>saved</h1>');
    await tester.tap(find.byKey(const Key('save-file')));
    await tester.pumpAndSettle();
    expect(utf8.decode(catalog.files['index.html']!), '<h1>saved</h1>');
  });

  testWidgets('open-with lists web preview for html', (tester) async {
    tester.view.physicalSize = const Size(1200, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: WorkspacePane(controller: workspace)),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('file-menu-index.html')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Open with…'));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('open-with-webPreview')), findsOneWidget);
    expect(find.byKey(const Key('open-with-textEditor')), findsOneWidget);
  });

  testWidgets('new file dialog creates index.html', (tester) async {
    tester.view.physicalSize = const Size(1200, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final catalog = MemoryWorkspaceCatalog();
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: WorkspacePane(controller: workspace)),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('explorer-new-file')));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('create-name-field')),
      'index.html',
    );
    await tester.tap(find.byKey(const Key('create-confirm')));
    await tester.pumpAndSettle();
    expect(catalog.files.containsKey('index.html'), isTrue);
    expect(find.text('index.html'), findsWidgets);
  });

  test('createCheckpoint posts label and restore refreshes files', () async {
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>old</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');
    final created = await workspace.createCheckpoint('before rewrite');
    expect(created?.label, 'before rewrite');
    expect(catalog.checkpointCalls, 1);
    expect(catalog.lastCheckpointLabel, 'before rewrite');
    expect(workspace.commits, isNotEmpty);

    await workspace.restoreCommit('abc1234');
    expect(catalog.restoreCalls, 1);
    expect(catalog.lastRestoreSha, 'abc1234');
    expect(catalog.listCalls, greaterThan(0));
  });

  testWidgets('checkpoint and restore dialogs call catalog', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'))
      ..commits.add(
        const GitCommit(
          sha: 'abc1234dead',
          message: 'agent: Landing (aaaaaaa)',
          label: 'before rewrite',
        ),
      );
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: WorkspacePane(controller: workspace)),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('checkpoint-button')));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('checkpoint-label-field')),
      'before rewrite',
    );
    await tester.tap(find.byKey(const Key('checkpoint-confirm')));
    await tester.pumpAndSettle();
    expect(catalog.checkpointCalls, 1);

    await tester.tap(find.byKey(const Key('history-button')));
    await tester.pumpAndSettle();
    expect(find.text('before rewrite'), findsWidgets);
    await tester.tap(find.byKey(const Key('diff-abc1234dead')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('diff-body')), findsOneWidget);
    expect(find.textContaining('<h1>new</h1>'), findsWidgets);
    await tester.tap(find.text('Close').last);
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('restore-abc1234dead')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('restore-confirm')));
    await tester.pumpAndSettle();
    expect(catalog.restoreCalls, 1);
    expect(catalog.lastRestoreSha, 'abc1234dead');
  });

  testWidgets(
    'WorkspacePage lists files and the focused view without docking',
    (tester) async {
      tester.view.physicalSize = const Size(400, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final catalog = MemoryWorkspaceCatalog()
        ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
      final workspace = WorkspaceController(catalog: catalog);
      await workspace.setProjectId('proj_1');

      await tester.pumpWidget(
        MaterialApp(home: WorkspacePage(controller: workspace)),
      );
      await tester.pumpAndSettle();

      expect(find.byType(Docking), findsNothing);
      expect(find.byKey(const Key('file-explorer')), findsOneWidget);
      expect(find.byType(DockViewBody), findsNothing);

      await tester.tap(find.byKey(const Key('file-row-index.html')));
      await tester.pumpAndSettle();

      expect(find.byType(Docking), findsNothing);
      expect(find.byKey(const Key('tab-view-1')), findsOneWidget);
      expect(find.byType(DockViewBody), findsOneWidget);
      expect(find.text('index.html · Editor'), findsOneWidget);
    },
  );

  testWidgets('closing a dirty mobile tab asks save, discard, or cancel', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(400, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final catalog = MemoryWorkspaceCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final workspace = WorkspaceController(catalog: catalog);
    await workspace.setProjectId('proj_1');
    await workspace.openDefault('index.html');

    await tester.pumpWidget(
      MaterialApp(home: WorkspacePage(controller: workspace)),
    );
    await tester.pumpAndSettle();

    workspace.documentFor('index.html')!.replaceText('<h1>edited</h1>');
    await tester.pump();

    await tester.tap(
      find.descendant(
        of: find.byKey(const Key('tab-view-1')),
        matching: find.byIcon(Icons.clear),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Save'), findsOneWidget);
    expect(find.text('Discard'), findsOneWidget);
    expect(find.text('Cancel'), findsOneWidget);
    expect(workspace.openViews, hasLength(1));

    await tester.tap(find.byKey(const Key('dirty-close-cancel')));
    await tester.pumpAndSettle();
    expect(workspace.openViews, hasLength(1));
    expect(utf8.decode(catalog.files['index.html']!), '<h1>hi</h1>');

    await tester.tap(
      find.descendant(
        of: find.byKey(const Key('tab-view-1')),
        matching: find.byIcon(Icons.clear),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('dirty-close-discard')));
    await tester.pumpAndSettle();
    expect(workspace.openViews, isEmpty);
    expect(utf8.decode(catalog.files['index.html']!), '<h1>hi</h1>');

    await workspace.openDefault('index.html');
    await tester.pumpAndSettle();
    workspace.documentFor('index.html')!.replaceText('<h1>saved</h1>');
    await tester.pump();
    final viewId = workspace.openViews.single.viewId;
    await tester.tap(
      find.descendant(
        of: find.byKey(Key('tab-$viewId')),
        matching: find.byIcon(Icons.clear),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('dirty-close-save')));
    await tester.pumpAndSettle();
    expect(workspace.openViews, isEmpty);
    expect(utf8.decode(catalog.files['index.html']!), '<h1>saved</h1>');
  });
}
