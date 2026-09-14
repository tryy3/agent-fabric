import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/chat_tab.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
  });

  test('defaults are collapsed', () async {
    final s = await ChatDisplaySettings.load();
    expect(s.thinking, VisibilityMode.collapsed);
    expect(s.stats, VisibilityMode.collapsed);
  });

  test('setThinking persists', () async {
    final s = await ChatDisplaySettings.load();
    await s.setThinking(VisibilityMode.hidden);
    final s2 = await ChatDisplaySettings.load();
    expect(s2.thinking, VisibilityMode.hidden);
  });

  testWidgets('Chat tab updates thinking visibility', (tester) async {
    final s = await ChatDisplaySettings.load();
    await tester.pumpWidget(MaterialApp(home: ChatTab(settings: s)));
    expect(find.text('Thinking'), findsOneWidget);
    await tester.tap(find.byKey(const Key('thinking-visibility')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Hidden').last);
    await tester.pumpAndSettle();
    expect(s.thinking, VisibilityMode.hidden);
  });

  testWidgets('hidden thinking still shows answer and caption', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              AgentBubble(
                thinkingMode: VisibilityMode.hidden,
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: 'hmm',
                ),
              ),
              AgentBubble(
                statsMode: VisibilityMode.hidden,
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.message,
                  text: 'hello',
                  model: 'm1',
                  providerName: 'Local',
                  predictedPerSecond: 35.5,
                ),
              ),
            ],
          ),
        ),
      ),
    );
    expect(find.text('Thinking'), findsNothing);
    expect(find.text('Stats'), findsNothing);
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('m1'), findsOneWidget);
    expect(find.textContaining('Local'), findsOneWidget);
    expect(find.textContaining('35.5'), findsOneWidget);
  });
}
