import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/chat_composer.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_inspector.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/shell/project_context_bar.dart';
import 'package:agent_fabric_client/shell/project_tabs_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:acpd/acpd.dart';
import 'package:docking/docking.dart' show MultiSplitView;
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _FakeConn implements AgentSessionApi {
  @override
  List<ModelOption> modelOptions = const [];
  @override
  String? currentModel;
  @override
  Stream<void> get closed => const Stream.empty();
  @override
  Stream<AcpConnectionState> get connectionState =>
      Stream.value(AcpConnectionState.connected);
  @override
  Future<void> connect({Transport? transport}) async {}
  @override
  Future<void> startSession(String agentId, {String? threadId}) async {}
  @override
  Future<void> setModel(String modelId) async {}
  @override
  Future<void> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
  }) async {}
  @override
  Future<void> cancel() async {}
  @override
  Future<void> close() async {}
}

http.Response _json(String body, [int status = 200]) {
  return http.Response(
    body,
    status,
    headers: {'content-type': 'application/json'},
  );
}

CatalogClient _rawCatalog() {
  const threadJson = '''
{
  "id": "th_1",
  "title": "T",
  "titleSource": "user",
  "messageCount": 2,
  "viewModeId": "raw",
  "projectId": "p1",
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z"
}
''';
  return CatalogClient(
    baseUri: Uri.parse('http://catalog.test'),
    httpClient: MockClient((request) async {
      final path = request.url.path;
      if (path.endsWith('/captures')) {
        return _json('[]');
      }
      if (path == '/v1/agents' ||
          path == '/v1/providers' ||
          path == '/v1/projects') {
        return _json('[]');
      }
      if (path == '/v1/threads') {
        return _json('[$threadJson]');
      }
      if (path == '/v1/threads/th_1') {
        return _json('''
{
  "id": "th_1",
  "title": "T",
  "titleSource": "user",
  "messageCount": 2,
  "viewModeId": "raw",
  "projectId": "p1",
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z",
  "messages": [
    {
      "id": "msg_u",
      "role": "user",
      "content": "hi",
      "position": 0,
      "createdAt": "2026-01-01T00:00:00Z",
      "parts": []
    },
    {
      "id": "msg_a",
      "role": "assistant",
      "content": "hello",
      "position": 1,
      "createdAt": "2026-01-01T00:00:01Z",
      "parts": [{"type":"message","text":"hello"}]
    }
  ]
}
''');
      }
      if (path.contains('/exporters') || path.contains('/settings')) {
        return _json('{}');
      }
      return _json('[]');
    }),
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('latestAssistantCatalogMessageId picks last assistant id', () {
    final id = latestAssistantCatalogMessageId([
      const ChatBubble(kind: ChatBubbleKind.user, text: 'hi'),
      const ChatBubble(
        kind: ChatBubbleKind.message,
        text: 'a',
        catalogMessageId: 'msg_1',
      ),
      const ChatBubble(
        kind: ChatBubbleKind.message,
        text: 'b',
        catalogMessageId: 'msg_2',
      ),
    ]);
    expect(id, 'msg_2');
  });

  test('hopRequestLabel is 1-based chronological; display is newest-first', () {
    expect(hopRequestLabel(0), 'R1');
    expect(hopRequestLabel(1), 'R2');
    expect(hopRequestLabel(3), 'R4');
    expect(hopRequestDisplayOrder(4), [3, 2, 1, 0]);
    expect(hopRequestDisplayOrder(4).map(hopRequestLabel).toList(), [
      'R4',
      'R3',
      'R2',
      'R1',
    ]);
  });

  testWidgets('Raw mode surface toggle lives in context bar', (tester) async {
    tester.view.physicalSize = const Size(1600, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = _rawCatalog();
    final controller = ChatController(session: _FakeConn(), catalog: catalog);
    addTearDown(controller.dispose);
    await controller.connect();
    await controller.selectThread('th_1');
    final tabs = ProjectTabsController();
    addTearDown(tabs.dispose);
    final dock = DockLayoutController();
    addTearDown(dock.dispose);
    final display = await ChatDisplaySettings.load();

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: Column(
            children: [
              ProjectContextBar(
                controller: controller,
                catalog: catalog,
                dock: dock,
                tabs: tabs,
                onSelectProject: (_) {},
                onCloseProjectTab: (_) {},
                onOpenSettings: () {},
              ),
              Expanded(
                child: ChatScreen(
                  controller: controller,
                  displaySettings: display,
                ),
              ),
            ],
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('chat-surface-chat')), findsOneWidget);
    expect(find.byKey(const Key('chat-surface-inspector')), findsOneWidget);
    expect(find.byKey(const Key('chat-surface-split')), findsOneWidget);
    expect(find.byKey(const Key('chat-surface-header')), findsOneWidget);
    expect(find.byKey(const Key('view-mode-menu')), findsOneWidget);

    Finder surfaceStack() => find.descendant(
      of: find.byType(ChatInspectorHost),
      matching: find.byType(IndexedStack),
    );
    int stackIndex() => tester.widget<IndexedStack>(surfaceStack()).index ?? -1;
    expect(stackIndex(), 0);

    await tester.tap(find.byKey(const Key('chat-surface-inspector')));
    await tester.pumpAndSettle();
    expect(controller.surfaceMode, ChatSurfaceMode.inspector);
    expect(stackIndex(), 1);
    expect(find.byKey(const Key('message-list')), findsNothing);
    expect(
      find.byKey(const Key('message-list'), skipOffstage: false),
      findsOneWidget,
    );
    expect(find.byType(ChatComposer), findsNothing);

    await tester.tap(find.byKey(const Key('chat-surface-split')));
    await tester.pumpAndSettle();
    expect(controller.surfaceMode, ChatSurfaceMode.split);
    expect(find.byType(MultiSplitView), findsOneWidget);
    expect(find.byType(ChatComposer), findsOneWidget);

    await tester.tap(find.byKey(const Key('chat-surface-chat')));
    await tester.pumpAndSettle();
    expect(stackIndex(), 0);
    expect(find.byType(MultiSplitView), findsNothing);
  });

  testWidgets('Pretty mode hides Inspector chrome', (tester) async {
    const threadJson = '''
{
  "id": "th_1",
  "title": "T",
  "titleSource": "user",
  "messageCount": 1,
  "viewModeId": "pretty",
  "projectId": "p1",
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z"
}
''';
    final catalog = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient((request) async {
        final path = request.url.path;
        if (path == '/v1/agents' ||
            path == '/v1/providers' ||
            path == '/v1/projects') {
          return _json('[]');
        }
        if (path == '/v1/threads') {
          return _json('[$threadJson]');
        }
        if (path == '/v1/threads/th_1') {
          return _json('''
{
  "id": "th_1",
  "title": "T",
  "titleSource": "user",
  "messageCount": 1,
  "viewModeId": "pretty",
  "projectId": "p1",
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z",
  "messages": [
    {
      "id": "msg_u",
      "role": "user",
      "content": "hi",
      "position": 0,
      "createdAt": "2026-01-01T00:00:00Z",
      "parts": []
    }
  ]
}
''');
        }
        if (path.contains('/exporters') || path.contains('/settings')) {
          return _json('{}');
        }
        return _json('[]');
      }),
    );
    final controller = ChatController(session: _FakeConn(), catalog: catalog);
    await controller.connect();
    await controller.selectThread('th_1');
    final display = await ChatDisplaySettings.load();
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: ChatScreen(controller: controller, displaySettings: display),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('chat-surface-inspector')), findsNothing);
  });
}
