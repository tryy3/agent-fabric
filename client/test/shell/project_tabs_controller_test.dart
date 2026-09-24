import 'package:agent_fabric_client/shell/project_tabs_controller.dart';
import 'package:agent_fabric_client/shell/workspace_memory.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  group('ProjectTabsController', () {
    setUp(() {
      SharedPreferences.setMockInitialValues({});
    });

    test('open appends once and reports changes', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);

      expect(tabs.isEmpty, isTrue);
      expect(tabs.open('proj-a'), isTrue);
      expect(tabs.open('proj-a'), isFalse);
      expect(tabs.open('proj-b'), isTrue);
      expect(tabs.openProjectIds, ['proj-a', 'proj-b']);
      expect(tabs.isOpen('proj-a'), isTrue);
      expect(tabs.isOpen('proj-c'), isFalse);
      expect(tabs.isEmpty, isFalse);
    });

    test('open ignores empty ids', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);

      expect(tabs.open(''), isFalse);
      expect(tabs.openProjectIds, isEmpty);
    });

    test('close returns the right neighbor, else the left one', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);
      tabs
        ..open('proj-a')
        ..open('proj-b')
        ..open('proj-c');

      expect(tabs.close('proj-b'), 'proj-c');
      expect(tabs.openProjectIds, ['proj-a', 'proj-c']);
      expect(tabs.close('proj-c'), 'proj-a');
      expect(tabs.close('proj-a'), isNull);
      expect(tabs.isEmpty, isTrue);
    });

    test('close of an unopened id is a no-op', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);
      tabs.open('proj-a');

      expect(tabs.close('proj-b'), isNull);
      expect(tabs.openProjectIds, ['proj-a']);
    });

    test('retainProjects drops tabs for missing projects', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);
      tabs
        ..open('proj-a')
        ..open('proj-b')
        ..open('proj-c');

      tabs.retainProjects({'proj-a', 'proj-c'});

      expect(tabs.openProjectIds, ['proj-a', 'proj-c']);
    });

    test('close then open reopens at the end of the strip', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);
      tabs
        ..open('proj-a')
        ..open('proj-b');

      tabs.close('proj-a');
      tabs.open('proj-a');

      expect(tabs.openProjectIds, ['proj-b', 'proj-a']);
    });

    test('notifies listeners on open, close, and retain', () {
      final tabs = ProjectTabsController();
      addTearDown(tabs.dispose);
      var notifications = 0;
      tabs.addListener(() => notifications++);

      tabs.open('proj-a');
      tabs.open('proj-a');
      tabs.close('proj-a');
      tabs.close('proj-a');
      tabs.retainProjects({});

      expect(notifications, 2);
    });

    test('persists the strip through WorkspaceMemory', () async {
      final memory = WorkspaceMemory();
      final tabs = ProjectTabsController(memory: memory);
      tabs
        ..open('proj-a')
        ..open('proj-b');
      await pumpEventQueue();

      expect(
        await memory.openProjects(),
        ['proj-a', 'proj-b'],
      );

      tabs.close('proj-a');
      await pumpEventQueue();

      expect(await memory.openProjects(), ['proj-b']);
    });

    test('restore keeps saved order and merges live opens', () async {
      final memory = WorkspaceMemory();
      await memory.rememberOpenProjects(['proj-a', 'proj-b']);

      final tabs = ProjectTabsController(memory: memory);
      // Like app startup: the strip loads while the selected project opens.
      final restoring = tabs.restore();
      tabs.open('proj-b');
      tabs.open('proj-c');
      await restoring;

      expect(tabs.openProjectIds, ['proj-a', 'proj-b', 'proj-c']);
      await pumpEventQueue();
      expect(await memory.openProjects(), ['proj-a', 'proj-b', 'proj-c']);
    });

    test('restore does not resurrect tabs closed while it was in flight', () async {
      final memory = WorkspaceMemory();
      await memory.rememberOpenProjects(['proj-a', 'proj-b']);

      final tabs = ProjectTabsController(memory: memory);
      final restoring = tabs.restore();
      // The startup project opens before the persisted strip finishes
      // loading, and the user closes it inside that same window.
      tabs.open('proj-a');
      tabs.close('proj-a');
      await restoring;

      expect(tabs.openProjectIds, ['proj-b']);
      await pumpEventQueue();
      expect(await memory.openProjects(), ['proj-b']);
    });

    test('restore without persisted tabs leaves the strip alone', () async {
      final memory = WorkspaceMemory();
      final tabs = ProjectTabsController(memory: memory);
      tabs.open('proj-a');
      await tabs.restore();

      expect(tabs.openProjectIds, ['proj-a']);
    });

    test('restore without memory skips loading', () async {
      final tabs = ProjectTabsController();
      tabs.open('proj-a');
      await tabs.restore();

      expect(tabs.openProjectIds, ['proj-a']);
    });
  });
}
