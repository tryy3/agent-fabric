import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  int connectCalls = 0;
  bool failConnect = false;
  bool failStartSession = false;
  bool failSetModel = false;
  bool failSend = false;
  bool failCancel = false;
  Completer<void>? startHang;
  String? startHangThreadId;
  Completer<void>? sendHang;
  final List<String> prompts = [];
  final List<String> startSessionIds = [];
  final List<String?> startSessionThreadIds = [];
  int cancels = 0;
  final List<String> setModels = [];
  List<String> thoughtsToEmit = const [];
  List<AgentToolCallEvent> toolCallsToEmit = const [];
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
    connectCalls++;
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
    startSessionThreadIds.add(threadId);
    final hang = startHang;
    if (hang != null &&
        (startHangThreadId == null || startHangThreadId == threadId)) {
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
    final hang = sendHang;
    if (hang != null) {
      await hang.future;
    }
    if (failSend) {
      throw StateError('send failed');
    }
    for (final t in thoughtsToEmit) {
      onEvent(AgentThoughtDelta(t));
    }
    for (final toolCall in toolCallsToEmit) {
      onEvent(toolCall);
    }
    for (final c in chunksToEmit) {
      onEvent(AgentMessageDelta(c));
    }
    final usage = usageToEmit;
    if (usage != null) {
      onEvent(AgentUsageEvent(usage));
    }
  }

  @override
  Future<void> cancel() async {
    cancels++;
    if (failCancel) {
      throw StateError('cancel failed');
    }
    sendHang?.completeError(StateError('cancelled'));
  }

  @override
  Future<void> close() async {
    connected = false;
    currentState = AcpConnectionState.disconnected;
    _connectionState.add(currentState);
  }
}

class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents, {List<ThreadSummary>? threads})
    : threads = List.of(threads ?? const []),
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
  final Map<String, List<ThreadMessage>> messages = {};
  Object? createError;
  Object? renameError;
  Object? getThreadError;
  Completer<void>? getThreadHang;
  String? getThreadHangId;
  Completer<void>? listThreadsHang;
  Object? listAgentsError;
  Object? listThreadsError;
  int listAgentsCalls = 0;
  int listThreadsCalls = 0;
  bool failPatch = false;
  String? lastPatchViewModeId;

  @override
  Future<List<Agent>> listAgents() async {
    listAgentsCalls++;
    if (listAgentsError != null) {
      throw listAgentsError!;
    }
    return List.of(agents);
  }

  @override
  Future<List<ThreadSummary>> listThreads() async {
    listThreadsCalls++;
    if (listThreadsError != null) {
      throw listThreadsError!;
    }
    final hang = listThreadsHang;
    if (hang != null) {
      await hang.future;
    }
    return List.of(threads);
  }

  @override
  Future<ThreadSummary> createThread() async {
    if (createError != null) {
      throw createError!;
    }
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
    if (getThreadError != null) {
      throw getThreadError!;
    }
    final hang = getThreadHang;
    if (hang != null && (getThreadHangId == null || getThreadHangId == id)) {
      await hang.future;
    }
    final thread = threads.firstWhere((t) => t.id == id);
    final msgs = List<ThreadMessage>.of(messages[id] ?? const []);
    return ThreadDetail(
      thread: ThreadSummary(
        id: thread.id,
        title: thread.title,
        titleSource: thread.titleSource,
        agentId: thread.agentId,
        currentModel: thread.currentModel,
        messageCount: msgs.length,
        viewModeId: thread.viewModeId,
        createdAt: thread.createdAt,
        updatedAt: thread.updatedAt,
      ),
      messages: msgs,
    );
  }

  @override
  Future<ThreadSummary> renameThread(String id, String title) async {
    if (renameError != null) {
      throw renameError!;
    }
    final i = threads.indexWhere((t) => t.id == id);
    final old = threads[i];
    final updated = ThreadSummary(
      id: old.id,
      title: title,
      titleSource: 'user',
      agentId: old.agentId,
      currentModel: old.currentModel,
      messageCount: old.messageCount,
      viewModeId: old.viewModeId,
      createdAt: old.createdAt,
      updatedAt: DateTime.utc(2026, 9, 13, 15),
    );
    threads[i] = updated;
    return updated;
  }

  @override
  Future<ThreadSummary> patchThreadViewMode(
    String id,
    String? viewModeId,
  ) async {
    if (failPatch) {
      throw CatalogException(statusCode: 500, message: 'patch failed');
    }
    lastPatchViewModeId = viewModeId;
    final i = threads.indexWhere((t) => t.id == id);
    final old = threads[i];
    final updated = old.copyWith(viewModeId: viewModeId);
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
    providerName: 'Local',
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

ThreadSummary _thread({
  required String id,
  required String title,
  String titleSource = 'auto',
  String? agentId,
  int messageCount = 0,
  DateTime? createdAt,
  DateTime? updatedAt,
}) {
  final created = createdAt ?? DateTime.utc(2026, 9, 13);
  return ThreadSummary(
    id: id,
    title: title,
    titleSource: titleSource,
    agentId: agentId,
    messageCount: messageCount,
    createdAt: created,
    updatedAt: updatedAt ?? created,
  );
}

void main() {
  test('connect moves status to connected', () async {
    final fake = FakeConn();
    final c = ChatController(session: fake);
    expect(c.status, ChatStatus.disconnected);
    await c.connect();
    expect(c.status, ChatStatus.connected);
    expect(c.canSend, isFalse);
  });

  test('connect returns early while reconnecting', () async {
    final fake = FakeConn();
    final c = ChatController(session: fake);
    await c.connect();
    fake.emitState(AcpConnectionState.reconnecting);

    await c.connect();

    expect(fake.connectCalls, 1);
    expect(c.status, ChatStatus.reconnecting);
  });

  test('send appends user message and streams assistant text', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.send('hi');
    expect(c.messages.map((m) => m.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.message,
    ]);
    expect(c.messages[0].text, 'hi');
    expect(c.messages[1].text, 'hello');
    expect(fake.prompts, ['hi']);
  });

  test('send accumulates thought separately from assistant text', () async {
    final conn = FakeConn()
      ..thoughtsToEmit = ['why']
      ..chunksToEmit = ['hello']
      ..usageToEmit = const TurnUsage(
        predictedPerSecond: 35.5,
        deltas: 1,
        stopReason: 'end_turn',
      );
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.send('hi');
    expect(c.messages.map((m) => m.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
      ChatBubbleKind.stats,
    ]);
    expect(c.messages[0].text, 'hi');
    expect(c.messages[1].text, 'why');
    expect(c.messages[2].text, 'hello');
    expect(c.messages[2].model, 'm1');
    expect(c.messages[2].providerName, 'Local');
    expect(c.messages[2].predictedPerSecond, 35.5);
    expect(c.messages[3].usage?.predictedPerSecond, 35.5);
    expect(c.messages[3].stopReason, 'end_turn');
    expect(c.messages[1].streamingThought, isFalse);
  });

  test('two thought deltas stay one thought bubble', () async {
    final conn = FakeConn()
      ..thoughtsToEmit = ['why', ' not']
      ..chunksToEmit = ['hello'];
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.send('hi');
    expect(c.messages.where((m) => m.kind == ChatBubbleKind.thought).length, 1);
    expect(
      c.messages.firstWhere((m) => m.kind == ChatBubbleKind.thought).text,
      'why not',
    );
  });

  test('tool-call updates with the same id upsert one bubble', () async {
    final conn = FakeConn()
      ..toolCallsToEmit = const [
        AgentToolCallEvent(
          id: 'call_1',
          title: 'Read file',
          status: 'in_progress',
          rawInput: {'path': 'notes.txt'},
          rawOutput: null,
          inProgress: true,
        ),
        AgentToolCallEvent(
          id: 'call_1',
          title: null,
          status: 'completed',
          rawInput: null,
          rawOutput: {'content': 'hello'},
          inProgress: false,
        ),
      ]
      ..chunksToEmit = ['done'];
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');

    await c.send('read notes');

    final tools = c.messages
        .where((bubble) => bubble.kind == ChatBubbleKind.toolCall)
        .toList();
    expect(tools, hasLength(1));
    expect(tools.single.toolCallId, 'call_1');
    expect(tools.single.toolTitle, 'Read file');
    expect(tools.single.toolStatus, 'completed');
    expect(tools.single.toolInput, {'path': 'notes.txt'});
    expect(tools.single.toolOutput, {'content': 'hello'});
    expect(tools.single.streamingTool, isFalse);
    expect(c.messages.map((bubble) => bubble.kind), [
      ChatBubbleKind.user,
      ChatBubbleKind.toolCall,
      ChatBubbleKind.message,
    ]);
  });

  test('send refresh replaces live bubbles with persisted GET parts', () async {
    final conn = FakeConn()
      ..thoughtsToEmit = ['why']
      ..chunksToEmit = ['hello'];
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: conn, catalog: catalog);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    final threadId = c.selectedThreadId!;
    catalog.messages[threadId] = [
      ThreadMessage(
        id: 'm1',
        role: 'user',
        content: 'hi',
        position: 0,
        createdAt: DateTime.utc(2026, 9, 13),
      ),
      ThreadMessage(
        id: 'm2',
        role: 'assistant',
        content: 'persisted-hello',
        position: 1,
        createdAt: DateTime.utc(2026, 9, 13),
        thought: 'persisted-why',
        model: 'm1',
        providerName: 'Local',
      ),
    ];
    await c.send('hi');
    expect(c.messages.map((m) => m.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
    ]);
    expect(
      c.messages.firstWhere((m) => m.kind == ChatBubbleKind.thought).text,
      'persisted-why',
    );
    expect(
      c.messages.firstWhere((m) => m.kind == ChatBubbleKind.message).text,
      'persisted-hello',
    );
  });

  test(
    'stale send refresh GET does not overwrite newly selected thread',
    () async {
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(id: 'th_live', title: 'Live', agentId: 'ag-1'),
          _thread(id: 'th_other', title: 'Other'),
        ],
      );
      catalog.messages['th_live'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'old-user',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
        ThreadMessage(
          id: 'm2',
          role: 'assistant',
          content: 'old-assistant',
          position: 1,
          createdAt: DateTime.utc(2026, 9, 13),
          thought: 'old-thought',
        ),
      ];
      catalog.messages['th_other'] = [
        ThreadMessage(
          id: 'm3',
          role: 'user',
          content: 'other-hi',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      final c = ChatController(session: FakeConn(), catalog: catalog);
      await c.connect();
      expect(c.selectedThreadId, 'th_live');

      final hang = Completer<void>();
      catalog
        ..getThreadHang = hang
        ..getThreadHangId = 'th_live';

      final sendFuture = c.send('hi');
      await Future<void>.delayed(Duration.zero);

      await c.selectThread('th_other');
      expect(c.messages.single.text, 'other-hi');

      hang.complete();
      await sendFuture;

      expect(c.selectedThreadId, 'th_other');
      expect(c.messages.single.text, 'other-hi');
      expect(c.messages.any((m) => m.text == 'old-assistant'), isFalse);
    },
  );

  test(
    'stale send GET after reselecting a thread does not overwrite load',
    () async {
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(id: 'th_a', title: 'A', agentId: 'ag-1'),
          _thread(id: 'th_b', title: 'B'),
        ],
      );
      catalog.messages['th_a'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'stale-user',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
        ThreadMessage(
          id: 'm2',
          role: 'assistant',
          content: 'stale-assistant',
          position: 1,
          createdAt: DateTime.utc(2026, 9, 13),
          thought: 'stale-thought',
        ),
      ];
      catalog.messages['th_b'] = [
        ThreadMessage(
          id: 'm3',
          role: 'user',
          content: 'b-hi',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      final c = ChatController(session: FakeConn(), catalog: catalog);
      await c.connect();
      expect(c.selectedThreadId, 'th_a');

      final hang = Completer<void>();
      catalog
        ..getThreadHang = hang
        ..getThreadHangId = 'th_a';

      final sendFuture = c.send('hi');
      await Future<void>.delayed(Duration.zero);
      catalog.getThreadHang = null;

      await c.selectThread('th_b');
      expect(c.messages.single.text, 'b-hi');

      catalog.messages['th_a'] = [
        ThreadMessage(
          id: 'm4',
          role: 'user',
          content: 'fresh-user',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
        ThreadMessage(
          id: 'm5',
          role: 'assistant',
          content: 'fresh-assistant',
          position: 1,
          createdAt: DateTime.utc(2026, 9, 13),
          thought: 'fresh-thought',
        ),
      ];
      await c.selectThread('th_a');
      expect(c.messages.map((m) => m.kind).toList(), [
        ChatBubbleKind.user,
        ChatBubbleKind.thought,
        ChatBubbleKind.message,
      ]);
      expect(
        c.messages.firstWhere((m) => m.kind == ChatBubbleKind.message).text,
        'fresh-assistant',
      );
      expect(
        c.messages.firstWhere((m) => m.kind == ChatBubbleKind.thought).text,
        'fresh-thought',
      );

      catalog.messages['th_a'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'stale-user',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
        ThreadMessage(
          id: 'm2',
          role: 'assistant',
          content: 'stale-assistant',
          position: 1,
          createdAt: DateTime.utc(2026, 9, 13),
          thought: 'stale-thought',
        ),
      ];
      hang.complete();
      await sendFuture;

      expect(c.selectedThreadId, 'th_a');
      expect(
        c.messages.firstWhere((m) => m.kind == ChatBubbleKind.message).text,
        'fresh-assistant',
      );
      expect(
        c.messages.firstWhere((m) => m.kind == ChatBubbleKind.thought).text,
        'fresh-thought',
      );
      expect(c.messages.any((m) => m.text == 'stale-assistant'), isFalse);
    },
  );

  test('stale selectThread GET does not overwrite a newer load', () async {
    final catalog = FakeCatalog(
      [_agent('ag-1', 'Alpha')],
      threads: [
        _thread(id: 'th_a', title: 'A'),
        _thread(id: 'th_b', title: 'B'),
      ],
    );
    catalog.messages['th_a'] = [
      ThreadMessage(
        id: 'm1',
        role: 'user',
        content: 'a-hi',
        position: 0,
        createdAt: DateTime.utc(2026, 9, 13),
      ),
    ];
    catalog.messages['th_b'] = [
      ThreadMessage(
        id: 'm2',
        role: 'user',
        content: 'b-hi',
        position: 0,
        createdAt: DateTime.utc(2026, 9, 13),
      ),
    ];
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    expect(c.selectedThreadId, 'th_a');
    expect(c.messages.single.text, 'a-hi');

    final hang = Completer<void>();
    catalog
      ..getThreadHang = hang
      ..getThreadHangId = 'th_b';

    final slower = c.selectThread('th_b');
    await Future<void>.delayed(Duration.zero);

    await c.selectThread('th_a');
    expect(c.selectedThreadId, 'th_a');
    expect(c.messages.single.text, 'a-hi');

    hang.complete();
    await slower;

    expect(c.selectedThreadId, 'th_a');
    expect(c.messages.single.text, 'a-hi');
    expect(c.messages.any((m) => m.text == 'b-hi'), isFalse);
  });

  test('selectThread maps persisted parts onto ChatBubble', () async {
    final catalog = FakeCatalog(
      [_agent('ag-1', 'Alpha')],
      threads: [_thread(id: 'th_parts', title: 'Parts', agentId: 'ag-1')],
    );
    catalog.messages['th_parts'] = [
      ThreadMessage(
        id: 'm1',
        role: 'user',
        content: 'hi',
        position: 0,
        createdAt: DateTime.utc(2026, 9, 13),
      ),
      ThreadMessage(
        id: 'm2',
        role: 'assistant',
        content: 'hello',
        position: 1,
        createdAt: DateTime.utc(2026, 9, 13),
        thought: 'hmm',
        model: 'm1',
        providerName: 'Local',
        stopReason: 'end_turn',
        usage: const TurnUsage(predictedPerSecond: 35.5, deltas: 1),
      ),
    ];
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    await c.selectThread('th_parts');
    expect(
      c.messages.map((m) => m.kind).toList(),
      catalog.messages['th_parts']!
          .expand(bubblesFromThreadMessage)
          .map((m) => m.kind)
          .toList(),
    );
    expect(c.messages.map((m) => m.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
      ChatBubbleKind.stats,
    ]);
    expect(c.messages[0].text, 'hi');
    expect(c.messages[1].text, 'hmm');
    expect(c.messages[2].text, 'hello');
    expect(c.messages[2].providerName, 'Local');
    expect(c.messages[2].model, 'm1');
    expect(c.messages[3].stopReason, 'end_turn');
    expect(c.messages[3].usage?.predictedPerSecond, 35.5);
  });

  test('connect failure sets error status', () async {
    final fake = FakeConn()..failConnect = true;
    final c = ChatController(session: fake);
    await c.connect();
    expect(c.status, ChatStatus.error);
    expect(c.canSend, isFalse);
  });

  test('idle peer disconnect sets disconnected and clears canSend', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    expect(c.status, ChatStatus.connected);
    expect(c.canSend, isTrue);

    fake.simulateDisconnect();

    expect(c.status, ChatStatus.disconnected);
    expect(c.canSend, isFalse);
  });

  test(
    'connectionState reconnecting maps to ChatStatus.reconnecting',
    () async {
      final fake = FakeConn();
      final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      expect(c.canSend, isTrue);

      fake.emitState(AcpConnectionState.reconnecting);
      expect(c.status, ChatStatus.reconnecting);
      expect(c.canSend, isFalse);
      expect(c.canSelectAgent, isFalse);
      expect(c.canSelectModel, isFalse);

      fake.emitState(AcpConnectionState.connected);
      expect(c.status, ChatStatus.connected);
      expect(c.canSend, isTrue);
      expect(fake.startSessionIds, ['ag-1']);
      await Future<void>.delayed(Duration.zero);
      expect(catalog.listAgentsCalls, 2);
      expect(catalog.listThreadsCalls, 2);
    },
  );

  test('disconnect while idle keeps the selected thread', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [_thread(id: 'th-1', title: 'Thread', agentId: 'ag-1')],
      ),
    );
    await c.connect();
    final threadId = c.selectedThreadId;

    fake.emitState(AcpConnectionState.disconnected);
    fake.simulateDisconnect();

    expect(c.status, ChatStatus.disconnected);
    expect(c.selectedThreadId, threadId);
    expect(c.canSend, isFalse);
  });

  test('failed reconnect refresh keeps the previous catalog data', () async {
    final fake = FakeConn();
    final catalog = FakeCatalog(
      [_agent('ag-1', 'Alpha')],
      threads: [_thread(id: 'th-1', title: 'Thread', agentId: 'ag-1')],
    );
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    final previousAgents = List<Agent>.of(c.agents);
    final previousThreads = List<ThreadSummary>.of(c.threads);
    catalog
      ..listAgentsError = StateError('agents unavailable')
      ..listThreadsError = StateError('threads unavailable');

    fake.emitState(AcpConnectionState.reconnecting);
    fake.emitState(AcpConnectionState.connected);
    await Future<void>.delayed(Duration.zero);

    expect(c.status, ChatStatus.connected);
    expect(c.agents, previousAgents);
    expect(c.threads, previousThreads);
    expect(c.statusMessage, contains('unavailable'));
  });

  test('formatChatError includes RpcError data', () {
    final err = RpcError(
      code: -32603,
      message: 'Internal error',
      data: {'error': 'OpenAI HTTP 401 Unauthorized: Invalid token payload'},
    );
    expect(
      formatChatError(err),
      contains('OpenAI HTTP 401 Unauthorized: Invalid token payload'),
    );
  });

  test(
    'connect loads agents from catalog without starting a session',
    () async {
      final fake = FakeConn();
      final c = ChatController(
        session: fake,
        catalog: FakeCatalog([_agent('ag-1', 'Alpha'), _agent('ag-2', 'Beta')]),
      );
      await c.connect();
      expect(c.agents.map((a) => a.id).toList(), ['ag-1', 'ag-2']);
      expect(fake.startSessionIds, isEmpty);
      expect(c.canSend, isFalse);
    },
  );

  test(
    'connect selects most recently listed thread and loads messages',
    () async {
      final fake = FakeConn();
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(
            id: 'th_new',
            title: 'Newer',
            agentId: 'ag-1',
            messageCount: 1,
            updatedAt: DateTime.utc(2026, 9, 13, 12),
          ),
          _thread(
            id: 'th_old',
            title: 'Older',
            agentId: 'ag-1',
            updatedAt: DateTime.utc(2026, 9, 13, 10),
          ),
        ],
      );
      catalog.messages['th_new'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'hi',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      expect(c.selectedThreadId, 'th_new');
      expect(c.messages.single.text, 'hi');
      expect(fake.startSessionIds, ['ag-1']);
      expect(fake.startSessionThreadIds, ['th_new']);
    },
  );

  test('connect with no threads leaves empty state', () async {
    final c = ChatController(session: FakeConn(), catalog: FakeCatalog([]));
    await c.connect();
    expect(c.selectedThreadId, isNull);
    expect(c.canSend, isFalse);
    expect(c.canSelectAgent, isFalse);
  });

  test('createThread selects untitled and does not start session', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    expect(c.threads.first.title, 'Untitled');
    expect(c.selectedThreadId, c.threads.first.id);
    expect(c.messages, isEmpty);
    expect(fake.startSessionIds, isEmpty);
    expect(c.canSend, isFalse);
    expect(c.canSelectAgent, isTrue);
  });

  test(
    'selectAgent on thread passes threadId and does not clear loaded messages',
    () async {
      final fake = FakeConn();
      final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      await c.createThread();
      final id = c.selectedThreadId!;
      catalog.messages[id] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'hello',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      await c.selectThread(id);
      expect(c.messages.single.text, 'hello');

      await c.selectAgent('ag-1');
      expect(fake.startSessionIds, ['ag-1']);
      expect(fake.startSessionThreadIds, [id]);
      expect(c.selectedAgentId, 'ag-1');
      expect(c.messages.single.text, 'hello');
      expect(c.canSend, isTrue);
      expect(c.canSelectAgent, isFalse);
    },
  );

  test(
    'selectAgent starts session on a new thread without clearing transcript',
    () async {
      final fake = FakeConn();
      final c = ChatController(
        session: fake,
        catalog: FakeCatalog([_agent('ag-1', 'Alpha'), _agent('ag-2', 'Beta')]),
      );
      await c.connect();
      await c.createThread();
      expect(c.canSelectAgent, isTrue);

      await c.selectAgent('ag-1');
      expect(fake.startSessionIds, ['ag-1']);
      expect(fake.startSessionThreadIds, [c.selectedThreadId]);
      expect(c.selectedAgentId, 'ag-1');
      expect(c.canSend, isTrue);
      expect(c.currentModel, 'm1');
      expect(c.modelOptions.map((m) => m.id).toList(), ['m1', 'm2']);
      expect(c.canSelectAgent, isFalse);

      await c.send('keep me');
      expect(c.messages, isNotEmpty);

      await c.selectAgent('ag-2');
      expect(fake.startSessionIds, ['ag-1']);
      expect(c.selectedAgentId, 'ag-1');
      expect(c.messages, isNotEmpty);
    },
  );

  test(
    'failed startSession keeps selection consistent and allows retry',
    () async {
      final fake = FakeConn()..failStartSession = true;
      final c = ChatController(
        session: fake,
        catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
      );
      await c.connect();
      await c.createThread();

      await c.selectAgent('ag-1');
      expect(c.selectedAgentId, isNull);
      expect(c.canSend, isFalse);
      expect(c.canSelectAgent, isTrue);
      expect(c.currentModel, isNull);
      expect(c.modelOptions, isEmpty);
      expect(c.statusMessage, isNotNull);
      expect(fake.startSessionIds, ['ag-1']);

      fake.failStartSession = false;
      await c.selectAgent('ag-1');
      expect(c.selectedAgentId, 'ag-1');
      expect(c.canSend, isTrue);
      expect(c.currentModel, 'm1');
      expect(c.modelOptions.map((m) => m.id).toList(), ['m1', 'm2']);
      expect(fake.startSessionIds, ['ag-1', 'ag-1']);
      expect(c.canSelectAgent, isFalse);

      fake.failStartSession = true;
      await c.selectAgent('ag-2');
      expect(c.selectedAgentId, 'ag-1');
      expect(c.canSend, isTrue);
      expect(c.currentModel, 'm1');
      expect(fake.startSessionIds, ['ag-1', 'ag-1']);
    },
  );

  test('selectModel forwards setModel to the session', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.selectModel('m2');
    expect(fake.setModels, ['m2']);
    expect(c.currentModel, 'm2');
    expect(c.messages, isEmpty);
  });

  test('canSelectModel requires connected ready session', () async {
    final hang = Completer<void>();
    final fake = FakeConn()..startHang = hang;
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha'), _agent('ag-2', 'Beta')]),
    );
    await c.connect();
    await c.createThread();
    expect(c.canSelectModel, isFalse);
    expect(c.canSelectAgent, isTrue);

    final first = c.selectAgent('ag-1');
    expect(c.canSelectAgent, isFalse);
    expect(c.canSelectModel, isFalse);
    hang.complete();
    await first;
    expect(c.canSelectAgent, isFalse);
    expect(c.canSelectModel, isTrue);
  });

  test('selectModel failure keeps connected and sets statusMessage', () async {
    final fake = FakeConn()..failSetModel = true;
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.selectModel('m2');
    expect(c.status, ChatStatus.connected);
    expect(c.statusMessage, contains('setModel failed'));
    expect(c.canSend, isTrue);
    expect(c.canSelectModel, isTrue);
  });

  test(
    'selectThread cancels in-flight send and drops uncommitted bubbles',
    () async {
      final hang = Completer<void>();
      final fake = FakeConn()..sendHang = hang;
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(id: 'th_live', title: 'Live', agentId: 'ag-1'),
          _thread(id: 'th_other', title: 'Other'),
        ],
      );
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      expect(c.selectedThreadId, 'th_live');

      final sendFuture = c.send('hi');
      await Future<void>.delayed(Duration.zero);
      expect(c.messages, isNotEmpty);

      await c.selectThread('th_other');
      expect(fake.cancels, 1);
      expect(c.messages, isEmpty);
      await sendFuture;
      expect(c.messages, isEmpty);
    },
  );

  test('threadFilter is case-insensitive title contains', () async {
    final catalog = FakeCatalog(
      [_agent('ag-1', 'Alpha')],
      threads: [
        _thread(id: 'th_1', title: 'Foo Bar'),
        _thread(id: 'th_2', title: 'Baz'),
      ],
    );
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    expect(c.visibleThreads.map((t) => t.id).toList(), ['th_1', 'th_2']);

    c.setThreadFilter('foo');
    expect(c.visibleThreads.map((t) => t.id).toList(), ['th_1']);

    c.setThreadFilter('FOO');
    expect(c.visibleThreads.map((t) => t.id).toList(), ['th_1']);
  });

  test('renameThread updates list and sets user source', () async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    final id = c.selectedThreadId!;
    await c.renameThread(id, 'My chat');
    expect(c.threads.single.id, id);
    expect(c.threads.single.title, 'My chat');
    expect(c.threads.single.titleSource, 'user');
    expect(c.selectedThread?.title, 'My chat');
  });

  test('failed send drops uncommitted bubbles', () async {
    final fake = FakeConn()..failSend = true;
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await c.send('hi');
    expect(c.messages, isEmpty);
  });

  test(
    'first successful send sets list title from first eight words',
    () async {
      final c = ChatController(
        session: FakeConn(),
        catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
      );
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      await c.send('one two three four five six seven eight nine ten');
      expect(c.threads.single.title, 'one two three four five six seven eight');
      expect(c.threads.single.titleSource, 'auto');
    },
  );

  test(
    'second send keeps first-prompt auto-title when GET is still Untitled',
    () async {
      final c = ChatController(
        session: FakeConn(),
        catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
      );
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      await c.send('first prompt title words here extra');
      await c.send('second prompt should not retitle');
      expect(c.threads.single.title, 'first prompt title words here extra');
      expect(c.threads.single.titleSource, 'auto');
    },
  );

  test(
    'send after rename keeps user title when GET is still Untitled auto',
    () async {
      final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
      final c = ChatController(session: FakeConn(), catalog: catalog);
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      final id = c.selectedThreadId!;
      final pinned = catalog.threads.indexWhere((t) => t.id == id);
      catalog.threads[pinned] = _thread(
        id: id,
        title: catalog.threads[pinned].title,
        titleSource: catalog.threads[pinned].titleSource,
        agentId: 'ag-1',
      );
      await c.renameThread(id, 'My chat');
      final i = catalog.threads.indexWhere((t) => t.id == id);
      catalog.threads[i] = _thread(
        id: id,
        title: 'Untitled',
        titleSource: 'auto',
        agentId: 'ag-1',
      );
      final stale = await catalog.getThread(id);
      expect(stale.thread.title, 'Untitled');
      expect(stale.thread.titleSource, 'auto');
      expect(c.selectedThread?.title, 'My chat');
      expect(c.selectedThread?.titleSource, 'user');
      await c.send('later prompt must not clobber rename');
      expect(c.threads.single.title, 'My chat');
      expect(c.threads.single.titleSource, 'user');
    },
  );

  test(
    'selectThread keeps messageCount from loaded messages not list zero',
    () async {
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(
            id: 'th_new',
            title: 'Newer',
            agentId: 'ag-1',
            messageCount: 0,
          ),
        ],
      );
      catalog.messages['th_new'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'hi',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
        ThreadMessage(
          id: 'm2',
          role: 'assistant',
          content: 'hello',
          position: 1,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      final c = ChatController(session: FakeConn(), catalog: catalog);
      await c.connect();
      expect(c.selectedThread?.messageCount, 2);
    },
  );

  test('failed GET after successful send keeps committed bubbles', () async {
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    catalog.getThreadError = CatalogException(
      statusCode: 500,
      message: 'refresh failed',
    );
    await c.send('hi');
    expect(c.messages.map((m) => m.kind).toList(), [
      ChatBubbleKind.user,
      ChatBubbleKind.message,
    ]);
    expect(c.messages[0].text, 'hi');
    expect(c.messages[1].text, 'hello');
    expect(c.statusMessage, contains('refresh failed'));
  });

  test('createThread error keeps list empty and sets statusMessage', () async {
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')])
      ..createError = CatalogException(
        statusCode: 500,
        message: 'create failed',
      );
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    await c.createThread();
    expect(c.threads, isEmpty);
    expect(c.selectedThreadId, isNull);
    expect(c.statusMessage, contains('create failed'));
  });

  test('renameThread error keeps previous title', () async {
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    await c.createThread();
    catalog.renameError = CatalogException(
      statusCode: 500,
      message: 'rename failed',
    );
    final id = c.selectedThreadId!;
    await c.renameThread(id, 'My chat');
    expect(c.threads.single.title, 'Untitled');
    expect(c.statusMessage, contains('rename failed'));
  });

  test('selectThread 404 clears selection and refreshes list', () async {
    final catalog = FakeCatalog(
      [_agent('ag-1', 'Alpha')],
      threads: [
        _thread(id: 'th_live', title: 'Live', agentId: 'ag-1'),
        _thread(id: 'th_gone', title: 'Gone'),
      ],
    );
    catalog.messages['th_live'] = [
      ThreadMessage(
        id: 'm1',
        role: 'user',
        content: 'keep',
        position: 0,
        createdAt: DateTime.utc(2026, 9, 13),
      ),
    ];
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    expect(c.selectedThreadId, 'th_live');
    expect(c.messages, isNotEmpty);

    catalog.threads.removeWhere((t) => t.id == 'th_gone');
    catalog.getThreadError = CatalogException(
      statusCode: 404,
      message: 'thread not found',
    );
    await c.selectThread('th_gone');
    expect(c.selectedThreadId, isNull);
    expect(c.messages, isEmpty);
    expect(c.threads.map((t) => t.id).toList(), ['th_live']);
    expect(c.statusMessage, contains('thread not found'));
  });

  test(
    'stale selectThread 404 listThreads does not clear a newer selection',
    () async {
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(id: 'th_live', title: 'Live', agentId: 'ag-1'),
          _thread(id: 'th_gone', title: 'Gone'),
        ],
      );
      catalog.messages['th_live'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'keep',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      final c = ChatController(session: FakeConn(), catalog: catalog);
      await c.connect();
      expect(c.selectedThreadId, 'th_live');

      final hang = Completer<void>();
      catalog
        ..getThreadError = CatalogException(
          statusCode: 404,
          message: 'thread not found',
        )
        ..listThreadsHang = hang;

      final stale = c.selectThread('th_gone');
      await Future<void>.delayed(Duration.zero);

      catalog.getThreadError = null;
      await c.selectThread('th_live');
      expect(c.selectedThreadId, 'th_live');
      expect(c.messages.single.text, 'keep');

      hang.complete();
      await stale;

      expect(c.selectedThreadId, 'th_live');
      expect(c.messages.single.text, 'keep');
      expect(c.selectedAgentId, 'ag-1');
      expect(c.statusMessage, isNull);
    },
  );

  test(
    'stale selectThread startSession does not overwrite a newer load',
    () async {
      final hang = Completer<void>();
      final fake = FakeConn()
        ..startHang = hang
        ..startHangThreadId = 'th_b';
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha'), _agent('ag-2', 'Beta')],
        threads: [
          _thread(id: 'th_a', title: 'A', agentId: 'ag-1'),
          _thread(id: 'th_b', title: 'B', agentId: 'ag-2'),
        ],
      );
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      expect(c.selectedThreadId, 'th_a');
      expect(c.selectedAgentId, 'ag-1');
      expect(c.canSend, isTrue);

      final slower = c.selectThread('th_b');
      await Future<void>.delayed(Duration.zero);
      expect(c.selectedThreadId, 'th_b');

      await c.selectThread('th_a');
      expect(c.selectedThreadId, 'th_a');
      expect(c.selectedAgentId, 'ag-1');
      expect(c.canSend, isTrue);

      hang.complete();
      await slower;

      expect(c.selectedThreadId, 'th_a');
      expect(c.selectedAgentId, 'ag-1');
      expect(c.canSend, isTrue);
    },
  );

  test(
    'selectThread non-404 error keeps current selection and transcript',
    () async {
      final catalog = FakeCatalog(
        [_agent('ag-1', 'Alpha')],
        threads: [
          _thread(id: 'th_live', title: 'Live', agentId: 'ag-1'),
          _thread(id: 'th_other', title: 'Other'),
        ],
      );
      catalog.messages['th_live'] = [
        ThreadMessage(
          id: 'm1',
          role: 'user',
          content: 'keep',
          position: 0,
          createdAt: DateTime.utc(2026, 9, 13),
        ),
      ];
      final c = ChatController(session: FakeConn(), catalog: catalog);
      await c.connect();
      catalog.getThreadError = CatalogException(
        statusCode: 500,
        message: 'get failed',
      );
      await c.selectThread('th_other');
      expect(c.selectedThreadId, 'th_live');
      expect(c.messages.single.text, 'keep');
      expect(c.statusMessage, contains('get failed'));
    },
  );

  test('selectThread cancel failure stays on current thread', () async {
    final hang = Completer<void>();
    final fake = FakeConn()
      ..sendHang = hang
      ..failCancel = true;
    final catalog = FakeCatalog(
      [_agent('ag-1', 'Alpha')],
      threads: [
        _thread(id: 'th_live', title: 'Live', agentId: 'ag-1'),
        _thread(id: 'th_other', title: 'Other'),
      ],
    );
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    expect(c.selectedThreadId, 'th_live');

    final sendFuture = c.send('hi');
    await Future<void>.delayed(Duration.zero);
    expect(c.messages, isNotEmpty);

    await c.selectThread('th_other');
    expect(fake.cancels, 1);
    expect(c.selectedThreadId, 'th_live');
    expect(c.messages, isNotEmpty);
    expect(c.statusMessage, contains('cancel failed'));

    hang.complete();
    await sendFuture;
  });

  test('incomplete selected agent cannot send and reloadAgents does not startSession', () async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    expect(c.canSend, isTrue);

    catalog.agents
      ..clear()
      ..add(_incomplete('ag-1', 'Alpha'));
    await c.reloadAgents();
    expect(c.selectedAgentId, 'ag-1');
    expect(c.selectedAgentIsComplete, isFalse);
    expect(c.canSend, isFalse);
    expect(fake.startSessionIds, ['ag-1']);

    await c.selectAgent('ag-1');
    expect(fake.startSessionIds, ['ag-1']);
  });

  test('reloadAgents restores canSend when selection is repaired with startSession', () async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    expect(c.canSend, isTrue);
    expect(fake.startSessionIds, ['ag-1']);

    catalog.agents
      ..clear()
      ..add(_incomplete('ag-1', 'Alpha'));
    await c.reloadAgents();
    expect(c.canSend, isFalse);
    expect(fake.startSessionIds, ['ag-1']);

    catalog.agents
      ..clear()
      ..add(_agent('ag-1', 'Alpha'));
    await c.reloadAgents();
    expect(c.canSend, isTrue);
    expect(fake.startSessionIds, ['ag-1', 'ag-1']);
  });

  test(
    'reloadAgents does not startSession when agent stayed complete',
    () async {
      final fake = FakeConn();
      final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      await c.reloadAgents();
      expect(c.canSend, isTrue);
      expect(fake.startSessionIds, ['ag-1']);
    },
  );

  test(
    'reloadAgents does not change sessionReady while selectAgent is in flight',
    () async {
      final hang = Completer<void>();
      final fake = FakeConn()..startHang = hang;
      final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      await c.createThread();

      final pending = c.selectAgent('ag-1');
      expect(c.canSend, isFalse);
      await c.reloadAgents();
      expect(c.canSend, isFalse);
      expect(fake.startSessionIds, ['ag-1']);

      hang.complete();
      await pending;
      expect(c.canSend, isTrue);
      expect(fake.startSessionIds, ['ag-1']);
    },
  );

  test('reloadAgents keeps previous agents when listAgents fails', () async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    expect(c.agents.map((a) => a.id).toList(), ['ag-1']);

    catalog.listAgentsError = StateError('catalog down');
    await c.reloadAgents();
    expect(c.agents.map((a) => a.id).toList(), ['ag-1']);
    expect(c.statusMessage, contains('catalog down'));
  });

  test('setThreadViewMode patches and updates selected thread', () async {
    final fake = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: FakeConn(), catalog: fake);
    await c.connect();
    await c.createThread();
    final id = c.selectedThreadId!;
    await c.setThreadViewMode('detailed');
    expect(c.selectedThread?.viewModeId, 'detailed');
    expect(fake.lastPatchViewModeId, 'detailed');
  });

  test('setThreadViewMode reverts on error', () async {
    final fake = FakeCatalog([_agent('ag-1', 'Alpha')])..failPatch = true;
    final c = ChatController(session: FakeConn(), catalog: fake);
    await c.connect();
    await c.createThread();
    await expectLater(
      c.setThreadViewMode('detailed'),
      throwsA(isA<CatalogException>()),
    );
    expect(c.selectedThread?.viewModeId, isNull);
  });

  test(
    'reloadAgents with deleted selection keeps id and blocks send',
    () async {
      final fake = FakeConn();
      final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
      final c = ChatController(session: fake, catalog: catalog);
      await c.connect();
      await c.createThread();
      await c.selectAgent('ag-1');
      catalog.agents.clear();
      await c.reloadAgents();
      expect(c.selectedAgentId, 'ag-1');
      expect(c.selectedAgentMissing, isTrue);
      expect(c.canSend, isFalse);
    },
  );
}
