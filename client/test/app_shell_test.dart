import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/app_shell.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/thread_pane.dart';
import 'package:agent_fabric_client/dock/dock_view_body.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:docking/docking.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _FakeConn implements AgentSessionApi {
  int connects = 0;
  bool failConnect = false;
  List<String> thoughtsToEmit = const [];
  TurnUsage? usageToEmit;
  final _closed = StreamController<void>.broadcast(sync: true);
  final _connectionState = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );
  AcpConnectionState currentState = AcpConnectionState.disconnected;

  @override
  Stream<void> get closed => _closed.stream;

  @override
  Stream<AcpConnectionState> get connectionState => _connectionState.stream;

  @override
  Future<void> connect({Transport? transport}) async {
    connects++;
    if (failConnect) {
      throw StateError('offline');
    }
    currentState = AcpConnectionState.connected;
    _connectionState.add(currentState);
  }

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {}

  @override
  Future<void> setModel(String modelId) async {}

  @override
  List<ModelOption> get modelOptions => const [];

  @override
  String? get currentModel => null;

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
  }) async {
    for (final t in thoughtsToEmit) {
      onEvent(AgentThoughtDelta(t));
    }
    final usage = usageToEmit;
    if (usage != null) {
      onEvent(AgentUsageEvent(usage));
    }
  }

  @override
  Future<void> cancel() async {}

  @override
  Future<void> close() async {
    currentState = AcpConnectionState.disconnected;
    _connectionState.add(currentState);
  }
}

class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents)
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient(
          (_) async => http.Response(
            '[]',
            200,
            headers: {'content-type': 'application/json'},
          ),
        ),
      );

  final List<Agent> agents;

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);
}

Agent _agent(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 1,
    providerId: 'prov-1',
    defaultModel: 'm1',
    createdAt: now,
    updatedAt: now,
  );
}

CatalogClient _emptyCatalog() {
  return CatalogClient(
    baseUri: Uri.parse('http://catalog.test'),
    httpClient: MockClient(
      (_) async => http.Response(
        '[]',
        200,
        headers: {'content-type': 'application/json'},
      ),
    ),
  );
}

void _useDesktopSurface(WidgetTester tester) {
  tester.view.physicalSize = const Size(1400, 900);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}

void main() {
  late ChatDisplaySettings displaySettings;
  late AppearanceSettings appearanceSettings;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    displaySettings = await ChatDisplaySettings.load();
    appearanceSettings = await AppearanceSettings.load();
  });

  testWidgets('shows Chat and Settings destinations', (
    WidgetTester tester,
  ) async {
    _useDesktopSurface(tester);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(find.text('Chat'), findsOneWidget);
    expect(find.text('Settings'), findsOneWidget);
  });

  testWidgets('tapping Settings shows SettingsPage', (
    WidgetTester tester,
  ) async {
    _useDesktopSurface(tester);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.byType(SettingsPage), findsOneWidget);
    expect(find.text('Providers'), findsWidgets);
    expect(find.byType(ChatScreen, skipOffstage: false), findsOneWidget);
  });

  testWidgets('shows Offline badge and can open Settings', (tester) async {
    _useDesktopSurface(tester);
    final session = _FakeConn()..failConnect = true;
    final controller = ChatController(session: session);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(find.text('Offline'), findsOneWidget);
    expect(find.text("You're offline"), findsWidgets);

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.byType(SettingsPage), findsOneWidget);
    expect(find.text('Offline'), findsOneWidget);
  });

  testWidgets('returning to Chat does not reconnect ACP', (
    WidgetTester tester,
  ) async {
    _useDesktopSurface(tester);
    final session = _FakeConn();
    final controller = ChatController(session: session);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(session.connects, 1);
    expect(find.byType(ChatScreen), findsOneWidget);

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    expect(find.byType(SettingsPage), findsOneWidget);
    expect(find.byType(ChatScreen, skipOffstage: false), findsOneWidget);
    expect(session.connects, 1);

    await tester.tap(
      find.descendant(
        of: find.byType(NavigationRail),
        matching: find.text('Chat'),
      ),
    );
    await tester.pumpAndSettle();
    expect(session.connects, 1);
    expect(find.text('Agent Fabric'), findsOneWidget);
  });

  testWidgets('returning to Chat reloads agents without reconnect', (
    tester,
  ) async {
    _useDesktopSurface(tester);
    final session = _FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final controller = ChatController(session: session, catalog: catalog);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();
    expect(session.connects, 1);
    expect(controller.agents, hasLength(1));

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    catalog.agents.add(_agent('ag-2', 'Beta'));

    await tester.tap(
      find.descendant(
        of: find.byType(NavigationRail),
        matching: find.text('Chat'),
      ),
    );
    await tester.pumpAndSettle();
    expect(session.connects, 1);
    expect(controller.agents.map((a) => a.id), ['ag-1', 'ag-2']);
  });

  testWidgets('thread pane visible on Chat and hidden on Settings', (
    tester,
  ) async {
    _useDesktopSurface(tester);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(find.byType(ThreadPane), findsOneWidget);

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.byType(SettingsPage), findsOneWidget);
    expect(find.byType(ThreadPane), findsNothing);
    expect(find.byType(ThreadPane, skipOffstage: false), findsWidgets);

    await tester.tap(
      find.descendant(
        of: find.byType(NavigationRail),
        matching: find.text('Chat'),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(SettingsPage), findsNothing);
    expect(find.byType(ThreadPane), findsOneWidget);
    expect(find.byType(ChatScreen), findsOneWidget);
  });

  testWidgets('rail Files toggles files dock panel', (tester) async {
    _useDesktopSurface(tester);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(find.byKey(const Key('file-explorer')), findsOneWidget);
    await tester.tap(find.byKey(const Key('rail-files')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('file-explorer')), findsNothing);
    await tester.tap(find.byKey(const Key('rail-files')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('file-explorer')), findsOneWidget);
  });

  testWidgets('default dock order threads then files then chat', (
    tester,
  ) async {
    _useDesktopSurface(tester);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(find.byType(ThreadPane), findsOneWidget);
    expect(find.byKey(const Key('file-explorer')), findsOneWidget);
    expect(find.byType(ChatScreen), findsOneWidget);

    final threadX = tester.getTopLeft(find.byType(ThreadPane)).dx;
    final filesX = tester.getTopLeft(find.byKey(const Key('file-explorer'))).dx;
    final chatX = tester.getTopLeft(find.byType(ChatScreen)).dx;

    expect(threadX, lessThan(filesX));
    expect(filesX, lessThan(chatX));
  });

  testWidgets('closing a dirty dock document asks save, discard, or cancel', (
    tester,
  ) async {
    _useDesktopSurface(tester);
    final catalog = _WorkspaceShellCatalog()
      ..files['index.html'] = Uint8List.fromList(utf8.encode('<h1>hi</h1>'));
    final controller = ChatController(session: _FakeConn(), catalog: catalog);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('file-row-index.html')));
    await tester.pumpAndSettle();

    final body = tester.widget<DockViewBody>(find.byType(DockViewBody));
    body.controller.documentFor('index.html')!.replaceText('<h1>edited</h1>');
    await tester.pump();

    await tester.tap(_docTabClose('index.html · Editor'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('dirty-close-save')), findsOneWidget);
    expect(find.byKey(const Key('dirty-close-discard')), findsOneWidget);
    expect(find.byKey(const Key('dirty-close-cancel')), findsOneWidget);
    expect(find.byType(DockViewBody), findsOneWidget);

    await tester.tap(find.byKey(const Key('dirty-close-cancel')));
    await tester.pumpAndSettle();
    expect(find.byType(DockViewBody), findsOneWidget);
    expect(utf8.decode(catalog.files['index.html']!), '<h1>hi</h1>');

    await tester.tap(_docTabClose('index.html · Editor'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('dirty-close-save')));
    await tester.pumpAndSettle();
    expect(find.byType(DockViewBody), findsNothing);
    expect(utf8.decode(catalog.files['index.html']!), '<h1>edited</h1>');
  });

  test('dirty tab close leaves removal to the interceptor when it rejects', () {
    final item = DockingItem(
      id: 'doc:a.txt:textEditor',
      name: 'a',
      widget: const SizedBox(),
    );
    var intercepts = 0;
    var closes = 0;

    invokeDirtyDockTabClose(
      item: item,
      interceptItemClose: (_) {
        intercepts++;
        return false;
      },
      onItemClose: (_) => closes++,
    );

    expect(intercepts, 1);
    expect(closes, 0);
  });

  test(
    'dirty tab close uses onItemClose once when the interceptor allows it',
    () {
      final item = DockingItem(
        id: 'doc:a.txt:textEditor',
        name: 'a',
        widget: const SizedBox(),
      );
      final closed = <DockingItem>[];

      invokeDirtyDockTabClose(
        item: item,
        interceptItemClose: (_) => true,
        onItemClose: closed.add,
      );

      expect(closed, [item]);
    },
  );

  test('dirty tab close ignores a missing item', () {
    var intercepts = 0;

    invokeDirtyDockTabClose(
      item: null,
      interceptItemClose: (_) {
        intercepts++;
        return true;
      },
      onItemClose: (_) => fail('closed'),
    );

    expect(intercepts, 0);
  });
}

Finder _docTabClose(String label) {
  final tab = find.ancestor(
    of: find.text(label),
    matching: find.byWidgetPredicate(
      (widget) => widget.runtimeType.toString() == 'TabWidget',
    ),
  );
  // Dirty chrome hides the package close control and docking appends a
  // maximize button after custom tab buttons, so the last button is not close.
  return find.descendant(
    of: tab,
    matching: find.byTooltip('Close unsaved'),
  );
}

class _WorkspaceShellCatalog extends CatalogClient {
  _WorkspaceShellCatalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient(
          (_) async => http.Response(
            '[]',
            200,
            headers: {'content-type': 'application/json'},
          ),
        ),
      );

  final Map<String, Uint8List> files = {};

  @override
  Future<List<Project>> listProjects() async {
    final now = DateTime.utc(2026, 9, 23);
    return [
      Project(id: 'proj_1', name: 'Default', createdAt: now, updatedAt: now),
    ];
  }

  @override
  Future<FsListing> listProjectFs(String projectId, {String path = '/'}) async {
    return FsListing(
      path: path,
      entries: [
        for (final entry in files.entries)
          if (!entry.key.contains('/'))
            FsEntry(name: entry.key, isDir: false, size: entry.value.length),
      ],
    );
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
    files[path] = Uint8List.fromList(bytes);
  }
}
