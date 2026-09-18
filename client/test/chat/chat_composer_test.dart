import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_composer.dart'
    show ChatComposer, composerMinLines;
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  final List<String> prompts = [];
  final List<String> startSessionIds = [];
  List<String> chunksToEmit = ['hel', 'lo'];
  final _connectionState = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );
  AcpConnectionState currentState = AcpConnectionState.disconnected;

  @override
  List<ModelOption> modelOptions = const [];

  @override
  String? currentModel;

  @override
  Stream<void> get closed => const Stream.empty();

  @override
  Stream<AcpConnectionState> get connectionState => _connectionState.stream;

  @override
  Future<void> connect({Transport? transport}) async {
    connected = true;
    currentState = AcpConnectionState.connected;
    _connectionState.add(currentState);
  }

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {
    startSessionIds.add(agentId);
    modelOptions = const [
      ModelOption(id: 'm1', name: 'Model 1'),
      ModelOption(id: 'm2', name: 'Model 2'),
    ];
    currentModel = 'm1';
  }

  @override
  Future<void> setModel(String modelId) async {
    currentModel = modelId;
  }

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
  }) async {
    prompts.add(text);
    for (final c in chunksToEmit) {
      onEvent(AgentMessageDelta(c));
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

void main() {
  test('composerMinLines is roomy for empty threads', () {
    expect(composerMinLines(hasMessages: false, focused: false), 4);
    expect(composerMinLines(hasMessages: false, focused: true), 4);
  });

  test('composerMinLines is compact when messages exist and unfocused', () {
    expect(composerMinLines(hasMessages: true, focused: false), 1);
  });

  test('composerMinLines expands when focused with messages', () {
    expect(composerMinLines(hasMessages: true, focused: true), 3);
  });

  testWidgets('empty thread uses roomy minLines on the input', (tester) async {
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
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();

    final field = tester.widget<TextField>(
      find.byKey(const Key('composer-input')),
    );
    expect(field.minLines, 4);
    expect(field.maxLines, 8);
  });

  testWidgets('after a message, unfocused composer is compact', (tester) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.send('hi');

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();

    final field = tester.widget<TextField>(
      find.byKey(const Key('composer-input')),
    );
    expect(field.minLines, 1);
  });

  testWidgets('composer sends on send button when canSend', (tester) async {
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
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('composer-input')), 'hello');
    await tester.tap(find.byKey(const Key('composer-send')));
    await tester.pumpAndSettle();

    expect(fake.prompts, ['hello']);
    expect(find.text('hello'), findsNothing); // cleared
  });

  testWidgets('attach and mic are disabled with Coming soon tooltip', (
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
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();

    final attach = tester.widget<IconButton>(
      find.byKey(const Key('composer-attach')),
    );
    final mic = tester.widget<IconButton>(
      find.byKey(const Key('composer-mic')),
    );
    expect(attach.onPressed, isNull);
    expect(mic.onPressed, isNull);

    expect(find.byTooltip('Coming soon'), findsNWidgets(2));
  });

  testWidgets('Enter sends and Shift+Enter inserts newline', (tester) async {
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
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('composer-input')), 'line1');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(fake.prompts, ['line1']);

    await tester.enterText(find.byKey(const Key('composer-input')), 'a');
    await tester.sendKeyDownEvent(LogicalKeyboardKey.shift);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shift);
    await tester.pumpAndSettle();
    expect(fake.prompts, ['line1']); // no second send
    final field = tester.widget<TextField>(
      find.byKey(const Key('composer-input')),
    );
    expect(field.controller!.text.contains('\n'), isTrue);
  });
}
