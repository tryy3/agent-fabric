import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/acp/slash_commands.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_composer.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

const _commands = [
  SlashCommand(
    name: 'deep-research',
    description: 'Research a question and write a cited report',
    hint: '<question>',
  ),
  SlashCommand(name: 'hello', description: 'Says hello'),
];

class _Conn implements AgentSessionApi, SlashCommandApi {
  final prompts = <String>[];
  final _state = StreamController<AcpConnectionState>.broadcast(sync: true);
  final _commandsChanged = StreamController<void>.broadcast(sync: true);
  List<SlashCommand> _slash = const [];

  void advertise(List<SlashCommand> commands) {
    _slash = commands;
    _commandsChanged.add(null);
  }

  @override
  List<SlashCommand> get slashCommands => _slash;

  @override
  Stream<void> get slashCommandsChanged => _commandsChanged.stream;

  @override
  List<ModelOption> modelOptions = const [];
  @override
  String? currentModel;
  @override
  String? pinnedPrompt;
  @override
  Stream<void> get closed => const Stream.empty();
  @override
  Stream<AcpConnectionState> get connectionState => _state.stream;
  @override
  Future<void> connect({Transport? transport}) async {
    _state.add(AcpConnectionState.connected);
  }

  @override
  void retryNow() {}
  @override
  Future<void> startSession(String agentId, {String? threadId}) async {
    modelOptions = const [ModelOption(id: 'm1', name: 'Model 1')];
    currentModel = 'm1';
  }

  @override
  Future<void> setModel(String modelId) async {}
  @override
  Future<StopReason> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
    bool retryLatest = false,
  }) async {
    prompts.add(text);
    return StopReason.endTurn;
  }

  @override
  Future<void> cancel() async {}
  @override
  Future<void> close() async {}
}

class _Catalog extends CatalogClient {
  _Catalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('[]', 200)),
      );

  final _threads = <ThreadSummary>[];

  @override
  Future<List<Assistant>> listAssistants() async {
    final now = DateTime.utc(2026, 9, 12);
    return [
      Assistant(
        id: 'ag-1',
        name: 'Alpha',
        version: 1,
        inferenceConnectionId: 'prov-1',
        defaultModel: 'm1',
        createdAt: now,
        updatedAt: now,
      ),
    ];
  }

  @override
  Future<List<ThreadSummary>> listThreads({String? projectId}) async =>
      List.of(_threads);

  @override
  Future<ThreadSummary> createThread({String? projectId}) async {
    final t = ThreadSummary(
      id: 'th_1',
      title: 'Untitled',
      titleSource: 'auto',
      createdAt: DateTime.utc(2026, 9, 13),
      updatedAt: DateTime.utc(2026, 9, 13),
    );
    _threads.insert(0, t);
    return t;
  }

  @override
  Future<ThreadDetail> getThread(String id) async =>
      ThreadDetail(thread: _threads.first, messages: const []);
}

void main() {
  group('matchSlashCommands', () {
    test('matches a bare /prefix only', () {
      expect(matchSlashCommands(_commands, '/').length, 2);
      expect(
        matchSlashCommands(_commands, '/dee').single.name,
        'deep-research',
      );
      expect(
        matchSlashCommands(_commands, '/DEE').single.name,
        'deep-research',
      );
      expect(matchSlashCommands(_commands, '/zzz'), isEmpty);
      expect(matchSlashCommands(_commands, 'hello'), isEmpty);
      expect(matchSlashCommands(_commands, '/hello world'), isEmpty);
    });

    test('insert text ends with a space', () {
      expect(_commands.first.insertText, '/deep-research ');
    });
  });

  test('slashCommandsFrom maps and sorts an ACP update', () {
    final update = AvailableCommandsUpdate(
      availableCommands: const [
        AvailableCommand(name: 'b', description: 'B'),
        AvailableCommand(
          name: 'a',
          description: 'A',
          input: UnstructuredCommandInput(hint: '<x>'),
        ),
      ],
    );
    final commands = slashCommandsFrom(update);
    expect(commands.map((c) => c.name), ['a', 'b']);
    expect(commands.first.hint, '<x>');
    expect(commands.last.hint, isNull);
  });

  testWidgets('typing / shows the picker and a pick fills the field', (
    tester,
  ) async {
    final conn = _Conn();
    final c = ChatController(session: conn, catalog: _Catalog());
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    conn.advertise(_commands);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('slash-command-list')), findsNothing);

    await tester.enterText(find.byKey(const Key('composer-input')), '/');
    await tester.pump();
    expect(
      find.byKey(const Key('slash-command-deep-research')),
      findsOneWidget,
    );
    expect(find.byKey(const Key('slash-command-hello')), findsOneWidget);
    expect(find.text('<question>'), findsOneWidget);

    await tester.enterText(find.byKey(const Key('composer-input')), '/he');
    await tester.pump();
    expect(find.byKey(const Key('slash-command-deep-research')), findsNothing);

    await tester.tap(find.byKey(const Key('slash-command-hello')));
    await tester.pump();
    final field = tester.widget<TextField>(
      find.byKey(const Key('composer-input')),
    );
    expect(field.controller!.text, '/hello ');
    expect(find.byKey(const Key('slash-command-list')), findsNothing);
  });

  testWidgets('arrow keys move and Enter picks instead of sending', (
    tester,
  ) async {
    final conn = _Conn();
    final c = ChatController(session: conn, catalog: _Catalog());
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAssistant('ag-1');
    conn.advertise(_commands);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('composer-input')));
    await tester.enterText(find.byKey(const Key('composer-input')), '/');
    await tester.pump();

    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pump();

    final field = tester.widget<TextField>(
      find.byKey(const Key('composer-input')),
    );
    expect(field.controller!.text, '/hello ');
    expect(conn.prompts, isEmpty);
  });
}
