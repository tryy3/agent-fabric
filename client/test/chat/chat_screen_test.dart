import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_composer.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  bool failConnect = false;
  bool failStartSession = false;
  bool failSetModel = false;
  Completer<void>? startHang;
  final List<String> prompts = [];
  final List<String> startSessionIds = [];
  final List<String> setModels = [];
  Completer<void>? sendHang;
  List<String> thoughtsToEmit = const [];
  List<String> chunksToEmit = ['hel', 'lo'];
  TurnUsage? usageToEmit;
  final _closed = StreamController<void>.broadcast(sync: true);
  final _connectionState = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );
  AcpConnectionState currentState = AcpConnectionState.disconnected;

  @override
  List<ModelOption> modelOptions = const [];

  @override
  String? currentModel;

  @override
  Stream<void> get closed => _closed.stream;

  @override
  Stream<AcpConnectionState> get connectionState => _connectionState.stream;

  void emitState(AcpConnectionState state) {
    currentState = state;
    connected = state == AcpConnectionState.connected;
    _connectionState.add(state);
  }

  void simulateDisconnect() {
    emitState(AcpConnectionState.disconnected);
    _closed.add(null);
  }

  @override
  Future<void> connect({Transport? transport}) async {
    if (failConnect) {
      throw StateError('dial failed');
    }
    connected = true;
    currentState = AcpConnectionState.connected;
    _connectionState.add(currentState);
  }

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {
    startSessionIds.add(agentId);
    final hang = startHang;
    if (hang != null) {
      await hang.future;
    }
    if (failStartSession) {
      throw StateError('session failed');
    }
    modelOptions = const [
      ModelOption(id: 'm1', name: 'Model 1'),
      ModelOption(id: 'm2', name: 'Model 2'),
    ];
    currentModel = 'm1';
  }

  @override
  Future<void> setModel(String modelId) async {
    setModels.add(modelId);
    if (failSetModel) {
      throw StateError('setModel failed');
    }
    currentModel = modelId;
  }

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
  }) async {
    prompts.add(text);
    for (final t in thoughtsToEmit) {
      onEvent(AgentThoughtDelta(t));
    }
    for (final c in chunksToEmit) {
      onEvent(AgentMessageDelta(c));
    }
    final usage = usageToEmit;
    if (usage != null) {
      onEvent(AgentUsageEvent(usage));
    }
    final hang = sendHang;
    if (hang != null) {
      await hang.future;
    }
  }

  @override
  Future<void> cancel() async {}

  @override
  Future<void> close() async {
    connected = false;
    currentState = AcpConnectionState.disconnected;
    _connectionState.add(currentState);
  }
}

class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents)
    : threads = [],
      super(
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
  final List<ThreadSummary> threads;
  String? lastPatchViewModeId;

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);

  @override
  Future<List<ThreadSummary>> listThreads() async => List.of(threads);

  @override
  Future<ThreadSummary> createThread() async {
    final t = ThreadSummary(
      id: 'th_${threads.length + 1}',
      title: 'Untitled',
      titleSource: 'auto',
      createdAt: DateTime.utc(2026, 9, 13),
      updatedAt: DateTime.utc(2026, 9, 13),
    );
    threads.insert(0, t);
    return t;
  }

  @override
  Future<ThreadDetail> getThread(String id) async {
    final thread = threads.firstWhere((t) => t.id == id);
    return ThreadDetail(thread: thread, messages: const []);
  }

  @override
  Future<ThreadSummary> patchThreadViewMode(
    String id,
    String? viewModeId,
  ) async {
    lastPatchViewModeId = viewModeId;
    final i = threads.indexWhere((t) => t.id == id);
    final updated = threads[i].copyWith(viewModeId: viewModeId);
    threads[i] = updated;
    return updated;
  }
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

Agent _incomplete(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 2,
    providerId: null,
    defaultModel: null,
    createdAt: now,
    updatedAt: now,
  );
}

void main() {
  late ChatDisplaySettings displaySettings;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    displaySettings = await ChatDisplaySettings.load();
  });

  testWidgets('agent and model pickers live in the composer not the app bar', (
    tester,
  ) async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    final agent = find.byKey(const Key('agent-picker'));
    final model = find.byKey(const Key('model-picker'));
    expect(agent, findsOneWidget);
    expect(model, findsOneWidget);

    expect(
      find.descendant(of: find.byType(AppBar), matching: agent),
      findsNothing,
    );
    expect(
      find.descendant(of: find.byType(ChatComposer), matching: agent),
      findsOneWidget,
    );
    expect(
      find.descendant(of: find.byType(ChatComposer), matching: model),
      findsOneWidget,
    );
  });

  testWidgets('incomplete agent is labeled and not selectable', (tester) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _incomplete('ag-2', 'Work'),
      ]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('agent-picker')));
    await tester.pumpAndSettle();
    expect(find.text('Work — needs provider'), findsOneWidget);

    await tester.tap(find.text('Work — needs provider'));
    await tester.pumpAndSettle();
    expect(fake.startSessionIds, isEmpty);
    expect(c.selectedAgentId, isNull);
  });

  testWidgets('selected incomplete agent shows status and blocks send', (
    tester,
  ) async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    catalog.agents
      ..clear()
      ..add(_incomplete('ag-1', 'Alpha'));
    await c.reloadAgents();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('This agent needs a provider'), findsOneWidget);
    expect(
      tester.widget<TextField>(find.byKey(const Key('composer-input'))).enabled,
      isFalse,
    );
    expect(
      tester
          .widget<IconButton>(find.byKey(const Key('composer-send')))
          .onPressed,
      isNull,
    );
  });

  testWidgets('selected deleted agent shows leftover and blocks send', (
    tester,
  ) async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    catalog.agents.clear();
    await c.reloadAgents();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('This agent was deleted'), findsOneWidget);
    expect(find.text('(deleted)'), findsOneWidget);
    expect(
      tester.widget<TextField>(find.byKey(const Key('composer-input'))).enabled,
      isFalse,
    );
    expect(
      tester
          .widget<IconButton>(find.byKey(const Key('composer-send')))
          .onPressed,
      isNull,
    );
  });

  testWidgets('reconnecting state shows status and disables composer', (
    tester,
  ) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    fake.emitState(AcpConnectionState.reconnecting);
    await tester.pump();

    expect(find.text('Reconnecting…'), findsOneWidget);
    expect(
      tester.widget<TextField>(find.byKey(const Key('composer-input'))).enabled,
      isFalse,
    );
  });

  testWidgets(
    'thinking stays open while streaming then follows collapsed default',
    (tester) async {
      final conn = FakeConn()
        ..thoughtsToEmit = ['hmm']
        ..chunksToEmit = ['hello']
        ..sendHang = Completer<void>();
      final c = ChatController(
        session: conn,
        catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
      );
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: ChatScreen(controller: c, displaySettings: displaySettings),
        ),
      );
      await tester.pumpAndSettle();

      await tester.enterText(find.byKey(const Key('composer-input')), 'hi');
      await tester.tap(find.byKey(const Key('composer-send')));
      await tester.pump();
      expect(find.text('hmm'), findsNWidgets(2));
      conn.sendHang!.complete();
      await tester.pumpAndSettle();
      expect(find.text('hmm'), findsOneWidget);
      expect(find.text('hello'), findsOneWidget);
    },
  );

  testWidgets('thinking appears above the answer while streaming', (
    tester,
  ) async {
    final conn = FakeConn()
      ..thoughtsToEmit = ['hmm']
      ..chunksToEmit = ['hello']
      ..sendHang = Completer<void>();
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('composer-input')), 'hi');
    await tester.tap(find.byKey(const Key('composer-send')));
    await tester.pump();
    expect(find.text('hmm'), findsNWidgets(2));
    expect(find.text('hello'), findsOneWidget);
    expect(
      tester.getTopLeft(find.text('Thinking')).dy,
      lessThan(tester.getTopLeft(find.text('hello')).dy),
    );
    conn.sendHang!.complete();
    await tester.pumpAndSettle();
  });

  testWidgets(
    'stats bubble is not shown as a card; Stats action is on message',
    (tester) async {
      final conn = FakeConn()
        ..chunksToEmit = ['hello']
        ..usageToEmit = const TurnUsage(elapsedMs: 50, deltas: 1);
      final c = ChatController(
        session: conn,
        catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
      );
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: ChatScreen(controller: c, displaySettings: displaySettings),
        ),
      );
      await tester.pumpAndSettle();

      await tester.enterText(find.byKey(const Key('composer-input')), 'hi');
      await tester.tap(find.byKey(const Key('composer-send')));
      await tester.pumpAndSettle();

      expect(find.textContaining('elapsedMs:'), findsNothing);
      expect(find.byKey(const Key('stats-action')), findsOneWidget);
      await tester.tap(find.byKey(const Key('stats-action')));
      await tester.pumpAndSettle();
      expect(find.text('Elapsed time'), findsOneWidget);
      expect(find.text('50'), findsOneWidget);
    },
  );

  testWidgets('view mode menu toggles markdown in transcript', (
    tester,
  ) async {
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: FakeConn(), catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    c.messages.addAll(const [
      ChatBubble(kind: ChatBubbleKind.user, text: '**bold**'),
      ChatBubble(kind: ChatBubbleKind.message, text: '**bold**'),
    ]);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(MarkdownBody), findsNWidgets(2));
    expect(find.text('Pretty'), findsWidgets);

    await tester.tap(find.byKey(const Key('view-mode-menu')));
    await tester.pumpAndSettle();
    expect(find.text('Rendered markdown, quiet harness'), findsOneWidget);
    expect(find.text('Plain text, more inspectable'), findsOneWidget);
    expect(find.text('Use app default'), findsNothing);
    await tester.tap(find.text('Detailed').last);
    await tester.pumpAndSettle();

    expect(catalog.lastPatchViewModeId, 'detailed');
    expect(find.byType(MarkdownBody), findsNothing);
    expect(find.text('**bold**'), findsNWidgets(2));
    expect(find.text('Detailed'), findsWidgets);

    await tester.tap(find.byKey(const Key('view-mode-menu')));
    await tester.pumpAndSettle();
    expect(find.text('Use app default'), findsOneWidget);
    await tester.tap(find.text('Use app default'));
    await tester.pumpAndSettle();

    expect(catalog.lastPatchViewModeId, isNull);
    expect(c.selectedThread?.viewModeId, isNull);
    expect(find.byType(MarkdownBody), findsNWidgets(2));
    expect(find.text('Pretty'), findsWidgets);
  });

  testWidgets('message list and composer respect content width', (
    tester,
  ) async {
    await displaySettings.setContentWidth(560);
    final conn = FakeConn();
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('composer-input')), 'Hello');
    await tester.tap(find.byKey(const Key('composer-send')));
    await tester.pumpAndSettle();

    final listBox = tester.widget<ConstrainedBox>(
      find
          .ancestor(
            of: find.text('Hello'),
            matching: find.byType(ConstrainedBox),
          )
          .first,
    );
    expect(listBox.constraints.maxWidth, 560);
    final composerBox = tester.widget<ConstrainedBox>(
      find
          .ancestor(
            of: find.byKey(const Key('composer-input')),
            matching: find.byType(ConstrainedBox),
          )
          .first,
    );
    expect(composerBox.constraints.maxWidth, 560);
  });
}
