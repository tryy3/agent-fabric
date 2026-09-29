import 'package:agent_fabric_client/shell/project_document_ref.dart';
import 'package:agent_fabric_client/shell/workbench_state_store.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  test(
    'remembers the last thread and explorer expansion per project',
    () async {
      SharedPreferences.setMockInitialValues({});
      final memory = WorkbenchStateStore();

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
    final memory = WorkbenchStateStore();

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
    final memory = WorkbenchStateStore();

    expect(await memory.documents('proj-a'), isEmpty);
    await memory.rememberDocuments('proj-a', [
      const ProjectDocumentRef(
        path: 'src/a.txt',
        appId: ProjectFileAppId.textEditor,
        viewMode: EditorViewMode.split,
        focused: true,
      ),
      const ProjectDocumentRef(
        path: 'index.html',
        appId: ProjectFileAppId.webPreview,
      ),
    ]);
    await memory.rememberDocuments('proj-b', [
      const ProjectDocumentRef(
        path: 'readme.md',
        appId: ProjectFileAppId.textEditor,
      ),
    ]);

    final a = await memory.documents('proj-a');
    expect(a, hasLength(2));
    expect(a.first.path, 'src/a.txt');
    expect(a.first.appId, ProjectFileAppId.textEditor);
    expect(a.first.viewMode, EditorViewMode.split);
    expect(a.first.focused, isTrue);
    expect(a.last.path, 'index.html');
    expect(a.last.appId, ProjectFileAppId.webPreview);
    expect(a.last.focused, isFalse);

    final b = await memory.documents('proj-b');
    expect(b, hasLength(1));
    expect(b.single.path, 'readme.md');

    await memory.rememberDocuments('proj-a', const []);
    expect(await memory.documents('proj-a'), isEmpty);
  });

  test('migrates legacy workspace_* preference keys once', () async {
    SharedPreferences.setMockInitialValues({
      'workspace_open_projects_v1': ['legacy-proj'],
      'workspace_active_project_v1': 'legacy-proj',
      'workspace_thread_v1:legacy-proj': 'th-old',
      'workspace_expansion_v1:legacy-proj': ['lib'],
      'workspace_documents_v1:legacy-proj':
          '[{"path":"a.txt","appId":"textEditor"}]',
    });
    final store = WorkbenchStateStore();

    expect(await store.openProjects(), ['legacy-proj']);
    expect(await store.lastActiveProject(), 'legacy-proj');
    expect(await store.lastThread('legacy-proj'), 'th-old');
    expect(await store.expansion('legacy-proj'), ['lib']);
    final docs = await store.documents('legacy-proj');
    expect(docs.single.path, 'a.txt');

    final prefs = await SharedPreferences.getInstance();
    expect(prefs.containsKey('workspace_open_projects_v1'), isFalse);
    expect(prefs.containsKey(WorkbenchStateStore.openProjectsKey), isTrue);
  });
}
