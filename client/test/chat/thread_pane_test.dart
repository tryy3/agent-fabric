import 'dart:typed_data';

import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/thread_pane.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'chat_controller_test.dart' show FakeCatalog, FakeConn;

ThreadSummary _thread({
  required String id,
  required String title,
  int messageCount = 0,
  String projectId = '',
}) {
  final now = DateTime.utc(2026, 9, 13);
  return ThreadSummary(
    id: id,
    title: title,
    titleSource: 'auto',
    messageCount: messageCount,
    projectId: projectId,
    createdAt: now,
    updatedAt: now,
  );
}

Project _project({required String id, required String name}) {
  final now = DateTime.utc(2026, 9, 20);
  return Project(id: id, name: name, createdAt: now, updatedAt: now);
}

Future<void> _pumpPane(
  WidgetTester tester, {
  required ChatController controller,
  required ChatDisplaySettings displaySettings,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: Row(
          children: [
            ThreadPane(controller: controller),
            Expanded(
              child: ChatScreen(
                controller: controller,
                displaySettings: displaySettings,
              ),
            ),
          ],
        ),
      ),
    ),
  );
  await tester.pump();
  await tester.pump();
}

void main() {
  late ChatDisplaySettings displaySettings;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    displaySettings = await ChatDisplaySettings.load();
  });
  testWidgets('new thread button creates untitled row', (tester) async {
    final c = ChatController(session: FakeConn(), catalog: FakeCatalog([]));
    addTearDown(c.dispose);
    await c.connect();
    await _pumpPane(tester, controller: c, displaySettings: displaySettings);

    expect(find.text('Create a thread to start chatting'), findsOneWidget);

    await tester.tap(find.byKey(const Key('new-thread')));
    await tester.pumpAndSettle();
    expect(find.text('Untitled'), findsWidgets);
    expect(find.text('Create a thread to start chatting'), findsNothing);
  });

  testWidgets('filter hides non-matching titles', (tester) async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        threads: [
          _thread(id: 'th_alpha', title: 'Alpha notes', messageCount: 1),
          _thread(id: 'th_beta', title: 'Beta draft', messageCount: 1),
        ],
      ),
    );
    addTearDown(c.dispose);
    await c.connect();
    await _pumpPane(tester, controller: c, displaySettings: displaySettings);

    expect(find.text('Alpha notes'), findsOneWidget);
    expect(find.text('Beta draft'), findsOneWidget);

    await tester.enterText(find.byKey(const Key('thread-filter')), 'alpha');
    await tester.pumpAndSettle();

    expect(find.text('Alpha notes'), findsOneWidget);
    expect(find.text('Beta draft'), findsNothing);
  });

  testWidgets('overflow rename dialog patches title', (tester) async {
    final c = ChatController(session: FakeConn(), catalog: FakeCatalog([]));
    addTearDown(c.dispose);
    await c.connect();
    await _pumpPane(tester, controller: c, displaySettings: displaySettings);

    await tester.tap(find.byKey(const Key('new-thread')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('thread-overflow')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Rename'));
    await tester.pumpAndSettle();

    await tester.enterText(
      find.byKey(const Key('rename-thread-field')),
      'My chat',
    );
    await tester.tap(find.text('OK'));
    await tester.pumpAndSettle();

    expect(find.text('My chat'), findsOneWidget);
    expect(c.threads.single.title, 'My chat');
  });

  testWidgets('project switcher lists threads for the selected project', (
    tester,
  ) async {
    final personal = _project(id: 'proj_personal', name: 'Personal');
    final landing = _project(id: 'proj_land', name: 'Landing');
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [personal, landing],
        threads: [
          _thread(
            id: 'th_p',
            title: 'Personal notes',
            messageCount: 1,
            projectId: personal.id,
          ),
          _thread(
            id: 'th_l',
            title: 'Landing chat',
            messageCount: 1,
            projectId: landing.id,
          ),
        ],
      ),
    );
    addTearDown(c.dispose);
    await c.connect();
    await _pumpPane(tester, controller: c, displaySettings: displaySettings);

    expect(find.text('Personal notes'), findsOneWidget);
    expect(find.text('Landing chat'), findsNothing);

    await tester.tap(find.byKey(const Key('project-switcher')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Landing').last);
    await tester.pumpAndSettle();

    expect(find.text('Landing chat'), findsOneWidget);
    expect(find.text('Personal notes'), findsNothing);
  });

  testWidgets('new project dialog creates and switches project', (
    tester,
  ) async {
    final c = ChatController(session: FakeConn(), catalog: FakeCatalog([]));
    addTearDown(c.dispose);
    await c.connect();
    await _pumpPane(tester, controller: c, displaySettings: displaySettings);

    await tester.tap(find.byKey(const Key('new-project')));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('new-project-field')),
      'Landing',
    );
    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(c.selectedProject?.name, 'Landing');
    expect(c.threads, isEmpty);
    expect(find.text('Landing'), findsWidgets);
  });

  testWidgets('export menu lists download and disabled GitHub', (tester) async {
    String? savedName;
    Uint8List? savedBytes;
    final catalog = FakeCatalog([]);
    final c = ChatController(
      session: FakeConn(),
      catalog: catalog,
      saveExport: (name, bytes) async {
        savedName = name;
        savedBytes = bytes;
      },
    );
    addTearDown(c.dispose);
    await c.connect();
    await _pumpPane(tester, controller: c, displaySettings: displaySettings);

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
