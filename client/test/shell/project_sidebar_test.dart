import 'dart:typed_data';

import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/shell/project_sidebar.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import '../chat/chat_controller_test.dart' show FakeCatalog, FakeConn;

ThreadSummary threadFixture({
  required String id,
  required String title,
  required String projectId,
}) {
  final now = DateTime.utc(2026, 9, 13, 16, 37);
  return ThreadSummary(
    id: id,
    title: title,
    titleSource: 'auto',
    messageCount: 1,
    projectId: projectId,
    createdAt: now,
    updatedAt: now,
  );
}

Project projectFixture({required String id, required String name}) {
  final now = DateTime.utc(2026, 9, 20);
  return Project(id: id, name: name, createdAt: now, updatedAt: now);
}

void main() {
  testWidgets('selecting a thread in another project switches the workspace', (
    tester,
  ) async {
    final personal = projectFixture(id: 'proj_personal', name: 'Default');
    final landing = projectFixture(id: 'proj_land', name: 'Landing');
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [personal, landing],
        threads: [
          threadFixture(
            id: 'th_p',
            title: 'Personal notes',
            projectId: personal.id,
          ),
          threadFixture(
            id: 'th_l',
            title: 'Landing chat',
            projectId: landing.id,
          ),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: ProjectSidebar(
            controller: controller,
            settingsActive: false,
            onOpenSettings: () {},
            onOpenWorkspace: () {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Personal notes'), findsOneWidget);
    expect(find.text('Landing chat'), findsNothing);

    // The chevron expands a group without leaving the active project.
    await tester.tap(find.byKey(const Key('project-toggle-proj_land')));
    await tester.pumpAndSettle();
    expect(find.text('Landing chat'), findsOneWidget);
    expect(controller.selectedProjectId, personal.id);

    await tester.tap(find.byKey(const Key('sidebar-thread-th_l')));
    await tester.pumpAndSettle();

    expect(controller.selectedProjectId, landing.id);
    expect(controller.selectedThreadId, 'th_l');
    expect(find.byKey(const Key('local-account')), findsOneWidget);
    expect(find.byKey(const Key('nav-settings')), findsOneWidget);
  });

  testWidgets('clicking a project row activates that project', (tester) async {
    final personal = projectFixture(id: 'proj_personal', name: 'Default');
    final landing = projectFixture(id: 'proj_land', name: 'Landing');
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [personal, landing],
        threads: [
          threadFixture(
            id: 'th_p',
            title: 'Personal notes',
            projectId: personal.id,
          ),
          threadFixture(
            id: 'th_l',
            title: 'Landing chat',
            projectId: landing.id,
          ),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: ProjectSidebar(
            controller: controller,
            settingsActive: false,
            onOpenSettings: () {},
            onOpenWorkspace: () {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(controller.selectedProjectId, personal.id);

    await tester.tap(find.byKey(const Key('project-row-proj_land')));
    await tester.pumpAndSettle();

    expect(controller.selectedProjectId, landing.id);
    // Activating a project expands its group so its threads stay visible.
    expect(find.text('Landing chat'), findsOneWidget);
  });

  testWidgets('new project dialog creates and switches project', (
    tester,
  ) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([]),
    );
    addTearDown(controller.dispose);
    await controller.connect();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: ProjectSidebar(
            controller: controller,
            settingsActive: false,
            onOpenSettings: () {},
            onOpenWorkspace: () {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('new-project')));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('new-project-field')),
      'Landing',
    );
    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(controller.selectedProject?.name, 'Landing');
    expect(find.text('Landing'), findsWidgets);
  });

  testWidgets('export menu lists download and disabled GitHub', (tester) async {
    String? savedName;
    Uint8List? savedBytes;
    final catalog = FakeCatalog([]);
    final controller = ChatController(
      session: FakeConn(),
      catalog: catalog,
      saveExport: (name, bytes) async {
        savedName = name;
        savedBytes = bytes;
      },
    );
    addTearDown(controller.dispose);
    await controller.connect();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: ProjectSidebar(
            controller: controller,
            settingsActive: false,
            onOpenSettings: () {},
            onOpenWorkspace: () {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('export-project')));
    await tester.pumpAndSettle();

    expect(find.text('Download zip'), findsOneWidget);
    expect(find.text('GitHub (coming soon)'), findsOneWidget);

    final github = tester.widget<PopupMenuItem<String>>(
      find.byKey(const Key('export-github')),
    );
    expect(github.enabled, isFalse);

    await tester.tap(find.byKey(const Key('export-download')));
    await tester.pumpAndSettle();

    expect(catalog.lastExportMethod, 'download');
    expect(savedName, 'Landing.zip');
    expect(savedBytes, catalog.exportBytes);
  });
}
