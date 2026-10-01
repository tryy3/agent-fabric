import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_composer.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:agent_fabric_client/chat/copy_action.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/shell/project_context_bar.dart';
import 'package:agent_fabric_client/shell/project_tabs_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:super_sliver_list/super_sliver_list.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  bool failConnect = false;
  bool failStartSession = false;
  bool failSetModel = false;
  bool failSend = false;
  Completer<void>? startHang;
  final List<String> prompts = [];
  final List<bool> retryLatestFlags = [];
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
  Future<StopReason> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
    bool retryLatest = false,
  }) async {
    prompts.add(text);
    retryLatestFlags.add(retryLatest);
    for (final t in thoughtsToEmit) {
      onEvent(AgentThoughtDelta(t));
    }
    for (final c in chunksToEmit) {
      onEvent(AgentMessageDelta(c));
    }
    if (failSend) {
      throw StateError('send failed');
    }
    final usage = usageToEmit;
    if (usage != null) {
      onEvent(AgentUsageEvent(usage));
    }
    final hang = sendHang;
    if (hang != null) {
      await hang.future;
    }
    return StopReason.endTurn;
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
  FakeCatalog(this.assistants)
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

  final List<Assistant> assistants;
  final List<ThreadSummary> threads;
  String? lastPatchViewModeId;

  @override
  Future<List<Assistant>> listAssistants() async => List.of(assistants);

  @override
  Future<List<Project>> listProjects() async => [_screenProject];

  @override
  Future<List<ThreadSummary>> listThreads({String? projectId}) async =>
      List.of(threads);

  @override
  Future<ThreadSummary> createThread({String? projectId}) async {
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

Assistant _assistant(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Assistant(
    id: id,
    name: name,
    version: 1,
    inferenceConnectionId: 'prov-1',
    defaultModel: 'm1',
    createdAt: now,
    updatedAt: now,
  );
}

Assistant _incomplete(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Assistant(
    id: id,
    name: name,
    version: 2,
    inferenceConnectionId: null,
    defaultModel: null,
    createdAt: now,
    updatedAt: now,
  );
}

final _screenProject = Project(
  id: 'proj_1',
  name: 'Default',
  createdAt: DateTime.utc(2026, 9, 12),
  updatedAt: DateTime.utc(2026, 9, 12),
);

void main() {
  late ChatDisplaySettings displaySettings;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    displaySettings = await ChatDisplaySettings.load();
  });

  tearDown(clearCopyToastForTest);

  testWidgets('agent and model pickers live in the composer not the app bar', (
    tester,
  ) async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
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
        _assistant('ag-1', 'Alpha'),
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
    expect(find.text('Work - needs connection'), findsOneWidget);

    await tester.tap(find.text('Work - needs connection'));
    await tester.pumpAndSettle();
    expect(fake.startSessionIds, isEmpty);
    expect(c.selectedAssistantId, isNull);
  });

  testWidgets('selected incomplete agent shows status and blocks send', (
    tester,
  ) async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_assistant('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    catalog.assistants
      ..clear()
      ..add(_incomplete('ag-1', 'Alpha'));
    await c.reloadAssistants();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('This assistant needs a connection'), findsOneWidget);
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
    final catalog = FakeCatalog([_assistant('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    catalog.assistants.clear();
    await c.reloadAssistants();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('This assistant was deleted'), findsOneWidget);
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
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    fake.emitState(AcpConnectionState.reconnecting);
    await tester.pump();

    expect(find.text('Reconnecting...'), findsOneWidget);
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
        catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
      );
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAssistant('ag-1');

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
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
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
        catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
      );
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAssistant('ag-1');
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

  testWidgets('message list omits stats bubbles as children', (tester) async {
    final conn = FakeConn()
      ..chunksToEmit = ['hello']
      ..usageToEmit = const TurnUsage(elapsedMs: 50, deltas: 1);
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
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

    // Controller still has a stats bubble in the model.
    expect(c.messages.where((m) => m.kind == ChatBubbleKind.stats), isNotEmpty);

    expect(find.byKey(const Key('message-list')), findsOneWidget);
    expect(find.byType(SuperListView), findsOneWidget);
    expect(
      c.messages.where((m) => m.kind != ChatBubbleKind.stats).length,
      c.messages.length -
          c.messages.where((m) => m.kind == ChatBubbleKind.stats).length,
    );
    // Visible rows only: user + message (+ thought/tool if present); stats still in model.
    expect(c.messages.where((m) => m.kind == ChatBubbleKind.stats), isNotEmpty);
    expect(find.byKey(const Key('stats-action')), findsOneWidget);
  });

  testWidgets('message list uses SuperListView', (tester) async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('message-list')), findsOneWidget);
    expect(
      tester
          .widget(find.byKey(const Key('message-list')))
          .runtimeType
          .toString(),
      contains('SuperListView'),
    );
  });

  testWidgets('view mode menu toggles markdown in transcript', (tester) async {
    tester.view.physicalSize = const Size(1600, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeCatalog([_assistant('ag-1', 'Alpha')]);
    final c = ChatController(session: FakeConn(), catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    c.messages.addAll(const [
      ChatBubble(kind: ChatBubbleKind.user, text: '**bold**'),
      ChatBubble(kind: ChatBubbleKind.message, text: '**bold**'),
    ]);
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    final dock = DockLayoutController();
    addTearDown(dock.dispose);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: Column(
            children: [
              ProjectContextBar(
                controller: c,
                catalog: catalog,
                dock: dock,
                tabs: tabs,
                onSelectProject: (_) {},
                onCloseProjectTab: (_) {},
                onOpenSettings: () {},
              ),
              Expanded(
                child: ChatScreen(
                  controller: c,
                  displaySettings: displaySettings,
                ),
              ),
            ],
          ),
        ),
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
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
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

    final listRight = tester
        .getTopRight(find.byKey(const Key('message-list')))
        .dx;
    expect(listRight - tester.getTopRight(find.text('Hello')).dx, lessThan(48));
    final composer = tester.getSize(find.byKey(const Key('composer-input')));
    expect(composer.width, greaterThan(560));
  });

  testWidgets('message list fills chat pane wider than content width', (
    tester,
  ) async {
    await displaySettings.setContentWidth(560);
    final conn = FakeConn();
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');

    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

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

    final list = find.byKey(const Key('message-list'));
    final listSize = tester.getSize(list);
    expect(listSize.width, greaterThan(560));
    expect(
      tester.getTopRight(list).dx - tester.getTopRight(find.text('Hello')).dx,
      lessThan(48),
    );

    final composer = tester.getSize(find.byKey(const Key('composer-input')));
    expect(composer.width, greaterThan(560));
  });

  testWidgets('user copy copies the prompt from the bubble footer', (
    tester,
  ) async {
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>?;
          copied.add(args?['text'] as String? ?? '');
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });

    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    c.messages.addAll(const [
      ChatBubble(kind: ChatBubbleKind.user, text: 'hello prompt'),
    ]);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('copy-user')), findsOneWidget);
    expect(find.text('hello prompt'), findsOneWidget);
    final promptBottom = tester.getBottomLeft(find.text('hello prompt')).dy;
    final copyTop = tester.getTopLeft(find.byKey(const Key('copy-user'))).dy;
    expect(copyTop, greaterThan(promptBottom));
    expect(copyTop - promptBottom, lessThan(48));
    await tester.tap(find.byKey(const Key('copy-user')));
    await tester.pumpAndSettle();
    expect(copied, ['hello prompt']);
    clearCopyToastForTest();
  });

  testWidgets('user footer shows locale timestamp next to copy', (
    tester,
  ) async {
    final when = DateTime(2026, 9, 9, 10, 40);
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    c.messages.add(
      ChatBubble(
        kind: ChatBubbleKind.user,
        text: 'hello prompt',
        createdAt: when,
      ),
    );

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        locale: const Locale('en', 'US'),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('copy-user')), findsOneWidget);
    expect(find.textContaining('Sep'), findsOneWidget);
    expect(find.textContaining('10:40'), findsOneWidget);
  });

  testWidgets('user footer omits timestamp when createdAt is null', (
    tester,
  ) async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    c.messages.addAll(const [
      ChatBubble(kind: ChatBubbleKind.user, text: 'hello prompt'),
    ]);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        locale: const Locale('en', 'US'),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('copy-user')), findsOneWidget);
    expect(find.textContaining('Sep'), findsNothing);
    expect(find.textContaining('10:40'), findsNothing);
  });

  testWidgets('error status line uses colorScheme.error', (tester) async {
    final fake = FakeConn()..failConnect = true;
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect(); // sets ChatStatus.error

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    final errorText = find.textContaining('Error:');
    expect(errorText, findsOneWidget);
    final style = tester.widget<Text>(errorText).style;
    expect(style?.color, AppTheme.light().colorScheme.error);
  });

  testWidgets('connected status line is not error-colored', (tester) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();
    final connected = tester.widget<Text>(find.text('Connected'));
    expect(connected.style?.color, isNot(AppTheme.light().colorScheme.error));
  });

  testWidgets('connected statusMessage status line uses colorScheme.error', (
    tester,
  ) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    c.statusMessage = 'setModel failed';
    c.notifyListeners();
    await tester.pump();

    final errorText = find.text('Error: setModel failed');
    expect(errorText, findsOneWidget);
    final style = tester.widget<Text>(errorText).style;
    expect(style?.color, AppTheme.light().colorScheme.error);
  });

  testWidgets(
    'deleted agent status line stays off error color with leftover status',
    (tester) async {
      final fake = FakeConn();
      final catalog = FakeCatalog([_assistant('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAssistant('ag-1');
      catalog.assistants.clear();
      await c.reloadAssistants();

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: ChatScreen(controller: c, displaySettings: displaySettings),
        ),
      );
      await tester.pumpAndSettle();

      c.status = ChatStatus.error;
      c.statusMessage = 'leftover failure';
      c.notifyListeners();
      await tester.pump();

      final deleted = tester.widget<Text>(
        find.text('This assistant was deleted'),
      );
      expect(find.textContaining('Error:'), findsNothing);
      expect(deleted.style?.color, isNot(AppTheme.light().colorScheme.error));
    },
  );

  testWidgets(
    'needs-provider status line stays off error color with leftover status',
    (tester) async {
      final fake = FakeConn();
      final catalog = FakeCatalog([_assistant('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAssistant('ag-1');
      catalog.assistants
        ..clear()
        ..add(_incomplete('ag-1', 'Alpha'));
      await c.reloadAssistants();

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: ChatScreen(controller: c, displaySettings: displaySettings),
        ),
      );
      await tester.pumpAndSettle();

      c.status = ChatStatus.error;
      c.statusMessage = 'leftover failure';
      c.notifyListeners();
      await tester.pump();

      final needsProvider = tester.widget<Text>(
        find.text('This assistant needs a connection'),
      );
      expect(find.textContaining('Error:'), findsNothing);
      expect(
        needsProvider.style?.color,
        isNot(AppTheme.light().colorScheme.error),
      );
    },
  );

  testWidgets('retry button on last user message triggers retryLatest', (
    tester,
  ) async {
    final fake = FakeConn()..chunksToEmit = ['first'];
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    await c.send('hi');

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('retry-user')), findsOneWidget);
    expect(find.byKey(const Key('copy-user')), findsOneWidget);

    fake.chunksToEmit = ['retry-answer'];
    await tester.tap(find.byKey(const Key('retry-user')));
    await tester.pumpAndSettle();

    expect(fake.retryLatestFlags, [false, true]);
    expect(find.text('retry-answer'), findsOneWidget);
  });

  testWidgets(
    'inference failure shows inline request failed and stays online',
    (tester) async {
      final fake = FakeConn()
        ..failSend = true
        ..chunksToEmit = ['partial'];
      final c = ChatController(
        session: fake,
        catalog: FakeCatalog([_assistant('ag-1', 'Alpha')]),
      );
      addTearDown(c.dispose);
      await c.connect();
      await c.createThread();
      await c.selectAssistant('ag-1');
      await c.send('hi');

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: ChatScreen(controller: c, displaySettings: displaySettings),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text("You're offline"), findsNothing);
      expect(find.text('Connected'), findsOneWidget);
      expect(find.byKey(const Key('activity-request-failed')), findsOneWidget);
      expect(find.text('Request failed'), findsOneWidget);
      expect(find.text('partial'), findsOneWidget);
      expect(find.byKey(const Key('retry-user')), findsOneWidget);
    },
  );
}
