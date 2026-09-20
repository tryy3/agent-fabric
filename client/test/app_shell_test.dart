import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/app_shell.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/thread_pane.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:agent_fabric_client/workspace/workspace_pane.dart';
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

    expect(find.byType(ThreadPane), findsNothing);
    expect(find.byType(SettingsPage), findsOneWidget);
  });

  testWidgets('Files toggle opens workspace pane', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
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

    expect(find.byKey(const Key('files-toggle')), findsOneWidget);
    expect(find.byType(WorkspacePane), findsNothing);

    await tester.tap(find.byKey(const Key('files-toggle')));
    await tester.pumpAndSettle();

    expect(find.byType(WorkspacePane), findsOneWidget);
    expect(find.byKey(const Key('file-explorer')), findsOneWidget);
  });
}
