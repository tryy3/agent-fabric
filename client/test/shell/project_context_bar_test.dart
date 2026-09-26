import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/shell/project_context_bar.dart';
import 'package:agent_fabric_client/shell/project_tabs_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import '../chat/chat_controller_test.dart' show FakeCatalog, FakeConn;
import 'project_sidebar_test.dart' show projectFixture;

Widget _wrap(Widget child) {
  return MaterialApp(
    theme: AppTheme.dark(),
    home: Scaffold(body: child),
  );
}

const _readFile = ToolDefinition(
  name: 'read_file',
  description: 'Read the contents of a file in the workspace.',
  parameters: {
    'type': 'object',
    'properties': {
      'path': {'type': 'string', 'description': 'Path under workspace'},
    },
    'required': ['path'],
  },
  requires: {'fs': true, 'exec': false},
  origin: 'sandbox',
);

void _wideSurface(WidgetTester tester) {
  tester.view.physicalSize = const Size(1600, 900);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}

void main() {
  testWidgets('Tools menu shows active definitions and stored allowlist', (
    tester,
  ) async {
    _wideSurface(tester);
    final now = DateTime.utc(2026, 9, 20);
    final catalog = FakeCatalog(
      [],
      projects: [
        Project(
          id: 'proj_1',
          name: 'Foobar',
          settings: {
            'tools': {
              'allow': ['read_file', 'custom_tool'],
            },
          },
          createdAt: now,
          updatedAt: now,
        ),
      ],
      planeTools: [_readFile],
    );
    final controller = ChatController(session: FakeConn(), catalog: catalog);
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController()..open(controller.selectedProjectId!);
    addTearDown(tabs.dispose);
    final dock = DockLayoutController();
    addTearDown(dock.dispose);

    var settingsOpened = false;
    await tester.pumpWidget(
      _wrap(
        ProjectContextBar(
          controller: controller,
          catalog: catalog,
          dock: dock,
          tabs: tabs,
          onSelectProject: (_) {},
          onCloseProjectTab: (_) {},
          onOpenSettings: () => settingsOpened = true,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('context-tools')));
    await tester.pumpAndSettle();

    expect(find.text('Active tools (1)'), findsOneWidget);
    expect(
      find.byKey(const Key('context-tool-name-read_file')),
      findsOneWidget,
    );
    expect(
      find.text('Read the contents of a file in the workspace.'),
      findsNothing,
    );
    expect(
      find.byKey(const Key('context-tool-params-read_file')),
      findsNothing,
    );

    await tester.tap(find.byKey(const Key('context-tool-expand-read_file')));
    await tester.pumpAndSettle();
    expect(
      find.byKey(const Key('context-tool-params-read_file')),
      findsOneWidget,
    );
    expect(
      find.textContaining('"description": "Read the contents of a file'),
      findsOneWidget,
    );
    expect(find.textContaining('"requires"'), findsOneWidget);

    expect(find.text('Stored allowlist'), findsOneWidget);
    expect(find.text('custom_tool'), findsOneWidget);
    expect(
      find.text('Stored only; tools are not filtered yet.'),
      findsOneWidget,
    );

    final edit = find.byKey(const Key('context-tools-edit-settings'));
    await tester.ensureVisible(edit);
    await tester.pumpAndSettle();
    await tester.tap(edit);
    await tester.pumpAndSettle();
    expect(settingsOpened, isTrue);
  });

  testWidgets('Tools menu shows empty active and empty allowlist copy', (
    tester,
  ) async {
    _wideSurface(tester);
    final catalog = FakeCatalog(
      [],
      projects: [projectFixture(id: 'proj_1', name: 'Empty')],
      planeTools: const [],
    );
    final controller = ChatController(session: FakeConn(), catalog: catalog);
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController()..open(controller.selectedProjectId!);
    addTearDown(tabs.dispose);
    final dock = DockLayoutController();
    addTearDown(dock.dispose);

    await tester.pumpWidget(
      _wrap(
        ProjectContextBar(
          controller: controller,
          catalog: catalog,
          dock: dock,
          tabs: tabs,
          onSelectProject: (_) {},
          onCloseProjectTab: (_) {},
          onOpenSettings: () {},
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('context-tools')));
    await tester.pumpAndSettle();

    expect(find.text('Active tools (0)'), findsOneWidget);
    expect(find.text('No tools available on this plane.'), findsOneWidget);
    expect(find.text('None'), findsOneWidget);
  });
}
