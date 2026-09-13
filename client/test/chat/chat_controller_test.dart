import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_message.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  bool failConnect = false;
  bool failStartSession = false;
  bool failSetModel = false;
  bool failSend = false;
  Completer<void>? startHang;
  Completer<void>? sendHang;
  final List<String> prompts = [];
  final List<String> startSessionIds = [];
  final List<String?> startSessionThreadIds = [];
  int cancels = 0;
  final List<String> setModels = [];
  List<String> chunksToEmit = ['hel', 'lo'];
  final _closed = StreamController<void>.broadcast(sync: true);

  @override
  List<ModelOption> modelOptions = const [];

  @override
  String? currentModel;

  @override
  Stream<void> get closed => _closed.stream;

  void simulateDisconnect() {
    connected = false;
    _closed.add(null);
  }

  @override
  Future<void> connect({Transport? transport}) async {
    if (failConnect) {
      throw StateError('dial failed');
    }
    connected = true;
  }

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {
    startSessionIds.add(agentId);
    startSessionThreadIds.add(threadId);
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
    required AgentChunkHandler onChunk,
  }) async {
    prompts.add(text);
    final hang = sendHang;
    if (hang != null) {
      await hang.future;
    }
    if (failSend) {
      throw StateError('send failed');
    }
    for (final c in chunksToEmit) {
      onChunk(c);
    }
  }

  @override
  Future<void> cancel() async {
    cancels++;
    sendHang?.completeError(StateError('cancelled'));
  }

  @override
  Future<void> close() async {
    connected = false;
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
    return ThreadDetail(
      thread: thread,
      messages: List.of(messages[id] ?? const []),
    );
  }

  @override
  Future<ThreadSummary> renameThread(String id, String title) async {
    final i = threads.indexWhere((t) => t.id == id);
    final old = threads[i];
    final updated = ThreadSummary(
      id: old.id,
      title: title,
      titleSource: 'user',
      agentId: old.agentId,
      currentModel: old.currentModel,
      messageCount: old.messageCount,
      createdAt: old.createdAt,
      updatedAt: DateTime.utc(2026, 9, 13, 15),
    );
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
    expect(c.messages.map((m) => m.role).toList(), [
      ChatRole.user,
      ChatRole.assistant,
    ]);
    expect(c.messages[0].text, 'hi');
    expect(c.messages[1].text, 'hello');
    expect(fake.prompts, ['hi']);
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

  test('connect loads agents from catalog without starting a session', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _agent('ag-2', 'Beta'),
      ]),
    );
    await c.connect();
    expect(c.agents.map((a) => a.id).toList(), ['ag-1', 'ag-2']);
    expect(fake.startSessionIds, isEmpty);
    expect(c.canSend, isFalse);
  });

  test('connect selects most recently listed thread and loads messages', () async {
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
  });

  test('connect with no threads leaves empty state', () async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([]),
    );
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
        catalog: FakeCatalog([
          _agent('ag-1', 'Alpha'),
          _agent('ag-2', 'Beta'),
        ]),
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
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _agent('ag-2', 'Beta'),
      ]),
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
}
