import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/shell/project_tab_strip.dart';
import 'package:agent_fabric_client/shell/project_tabs_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import '../chat/chat_controller_test.dart' show FakeCatalog, FakeConn;
import 'project_sidebar_test.dart' show projectFixture, threadFixture;

Widget _wrap(Widget child) {
  return MaterialApp(
    theme: AppTheme.dark(),
    home: Scaffold(body: child),
  );
}

ProjectTabStrip _strip(
  ChatController controller,
  ProjectTabsController tabs, {
  ValueChanged<String>? onSelect,
  ValueChanged<String>? onClose,
}) {
  return ProjectTabStrip(
    controller: controller,
    tabs: tabs,
    onSelectProject: onSelect ?? (_) {},
    onCloseProjectTab: onClose ?? (_) {},
  );
}

void main() {
  testWidgets('renders one tab per open project and marks the active one', (
    tester,
  ) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [
          projectFixture(id: 'proj_default', name: 'Default'),
          projectFixture(id: 'proj_land', name: 'Landing'),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    tabs
      ..open(controller.selectedProjectId!)
      ..open('proj_land');

    await tester.pumpWidget(
      _wrap(_strip(controller, tabs, onSelect: (_) => fail('selected'))),
    );

    expect(find.byKey(const Key('project-tab-proj_default')), findsOneWidget);
    expect(find.byKey(const Key('project-tab-proj_land')), findsOneWidget);
    expect(find.byKey(const Key('active-project-tab')), findsOneWidget);
    expect(
      find.descendant(
        of: find.byKey(const Key('project-tab-proj_default')),
        matching: find.byKey(const Key('active-project-tab')),
      ),
      findsOneWidget,
    );
    expect(find.byKey(const Key('no-project-tab')), findsNothing);
  });

  testWidgets('tapping a tab reports the project id', (tester) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [
          projectFixture(id: 'proj_default', name: 'Default'),
          projectFixture(id: 'proj_land', name: 'Landing'),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    tabs
      ..open(controller.selectedProjectId!)
      ..open('proj_land');

    final selected = <String>[];
    await tester.pumpWidget(
      _wrap(_strip(controller, tabs, onSelect: selected.add)),
    );

    await tester.tap(find.byKey(const Key('project-tab-proj_land')));
    await tester.pump();

    expect(selected, ['proj_land']);
  });

  testWidgets('close buttons report their tab id', (tester) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [
          projectFixture(id: 'proj_default', name: 'Default'),
          projectFixture(id: 'proj_land', name: 'Landing'),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    tabs
      ..open(controller.selectedProjectId!)
      ..open('proj_land');

    final closed = <String>[];
    await tester.pumpWidget(
      _wrap(_strip(controller, tabs, onClose: closed.add)),
    );

    await tester.tap(find.byKey(const Key('close-project-tab-proj_land')));
    await tester.pump();

    expect(closed, ['proj_land']);
  });

  testWidgets('plus button lists only unopened projects and selects one', (
    tester,
  ) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [
          projectFixture(id: 'proj_default', name: 'Default'),
          projectFixture(id: 'proj_land', name: 'Landing'),
          projectFixture(id: 'proj_bench', name: 'Benchmarks'),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    tabs.open(controller.selectedProjectId!);

    final selected = <String>[];
    await tester.pumpWidget(
      _wrap(_strip(controller, tabs, onSelect: selected.add)),
    );

    await tester.tap(find.byKey(const Key('new-project-tab')));
    await tester.pumpAndSettle();

    // Open projects are not offered again.
    expect(find.byKey(const Key('new-project-tab-proj_default')), findsNothing);
    expect(find.byKey(const Key('new-project-tab-proj_land')), findsOneWidget);
    expect(find.byKey(const Key('new-project-tab-proj_bench')), findsOneWidget);

    await tester.tap(find.byKey(const Key('new-project-tab-proj_land')));
    await tester.pumpAndSettle();

    expect(selected, ['proj_land']);
  });

  testWidgets('shows the empty state when no tabs are open', (tester) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [projectFixture(id: 'proj_default', name: 'Default')],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);

    await tester.pumpWidget(
      _wrap(_strip(controller, tabs, onSelect: (_) => fail('selected'))),
    );

    expect(find.byKey(const Key('no-project-tab')), findsOneWidget);
    expect(find.text('No project'), findsOneWidget);
    // The strip still offers to open the first project.
    expect(find.byKey(const Key('new-project-tab')), findsOneWidget);
  });

  testWidgets('threads opening a project adds its tab', (tester) async {
    final controller = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog(
        [],
        projects: [
          projectFixture(id: 'proj_default', name: 'Default'),
          projectFixture(id: 'proj_land', name: 'Landing'),
        ],
        threads: [
          threadFixture(
            id: 'th_p',
            title: 'Personal notes',
            projectId: 'proj_default',
          ),
          threadFixture(
            id: 'th_l',
            title: 'Landing chat',
            projectId: 'proj_land',
          ),
        ],
      ),
    );
    addTearDown(controller.dispose);
    await controller.connect();
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    tabs.open(controller.selectedProjectId!);

    await tester.pumpWidget(_wrap(_strip(controller, tabs)));
    expect(find.byKey(const Key('project-tab-proj_land')), findsNothing);

    // Opening a thread in another project is the tab-opening path the shell
    // wires to the sidebar.
    await controller.selectProject('proj_land');
    tabs.open('proj_land');
    await tester.pump();

    expect(find.byKey(const Key('project-tab-proj_land')), findsOneWidget);
  });
}
