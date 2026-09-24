import 'package:agent_fabric_client/shell/workspace_document_ref.dart';
import 'package:agent_fabric_client/shell/workspace_memory.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  test(
    'remembers the last thread and explorer expansion per project',
    () async {
      SharedPreferences.setMockInitialValues({});
      final memory = WorkspaceMemory();

      expect(await memory.lastThread('proj-a'), isNull);
      await memory.rememberThread('proj-a', 'th-1');
      await memory.rememberThread('proj-b', 'th-2');
      expect(await memory.lastThread('proj-a'), 'th-1');
      expect(await memory.lastThread('proj-b'), 'th-2');

      await memory.rememberExpansion('proj-a', ['src', 'docs']);
      expect(await memory.expansion('proj-a'), ['src', 'docs']);
      expect(await memory.expansion('proj-b'), isEmpty);
    },
  );

  test('remembers open project tabs and the active project', () async {
    SharedPreferences.setMockInitialValues({});
    final memory = WorkspaceMemory();

    expect(await memory.openProjects(), isEmpty);
    await memory.rememberOpenProjects(['proj-a', 'proj-b']);
    expect(await memory.openProjects(), ['proj-a', 'proj-b']);

    expect(await memory.lastActiveProject(), isNull);
    await memory.rememberActiveProject('proj-b');
    expect(await memory.lastActiveProject(), 'proj-b');

    await memory.forgetActiveProject();
    expect(await memory.lastActiveProject(), isNull);
    // The tab strip outlives the active project being cleared.
    expect(await memory.openProjects(), ['proj-a', 'proj-b']);
  });

  test('remembers open documents per project', () async {
    SharedPreferences.setMockInitialValues({});
    final memory = WorkspaceMemory();

    expect(await memory.documents('proj-a'), isEmpty);
    await memory.rememberDocuments('proj-a', [
      const WorkspaceDocumentRef(
        path: 'src/a.txt',
        appId: WorkspaceAppId.textEditor,
        viewMode: EditorViewMode.split,
        focused: true,
      ),
      const WorkspaceDocumentRef(
        path: 'index.html',
        appId: WorkspaceAppId.webPreview,
      ),
    ]);
    await memory.rememberDocuments('proj-b', [
      const WorkspaceDocumentRef(
        path: 'readme.md',
        appId: WorkspaceAppId.textEditor,
      ),
    ]);

    final a = await memory.documents('proj-a');
    expect(a, hasLength(2));
    expect(a.first.path, 'src/a.txt');
    expect(a.first.appId, WorkspaceAppId.textEditor);
    expect(a.first.viewMode, EditorViewMode.split);
    expect(a.first.focused, isTrue);
    expect(a.last.path, 'index.html');
    expect(a.last.appId, WorkspaceAppId.webPreview);
    expect(a.last.focused, isFalse);

    final b = await memory.documents('proj-b');
    expect(b, hasLength(1));
    expect(b.single.path, 'readme.md');

    await memory.rememberDocuments('proj-a', const []);
    expect(await memory.documents('proj-a'), isEmpty);
  });
}
