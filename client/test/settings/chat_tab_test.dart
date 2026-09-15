import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/chat_tab.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
  });

  test('defaults: thinking collapsed, contentWidth 720', () async {
    final s = await ChatDisplaySettings.load();
    expect(s.thinking, VisibilityMode.collapsed);
    expect(s.contentWidth, 720);
  });

  test('setThinking persists', () async {
    final s = await ChatDisplaySettings.load();
    await s.setThinking(VisibilityMode.hidden);
    final s2 = await ChatDisplaySettings.load();
    expect(s2.thinking, VisibilityMode.hidden);
  });

  test('setContentWidth persists and clamps', () async {
    final s = await ChatDisplaySettings.load();
    await s.setContentWidth(500); // below min
    expect(s.contentWidth, 560);
    await s.setContentWidth(2000); // above max
    expect(s.contentWidth, 1200);
    await s.setContentWidth(800);
    final s2 = await ChatDisplaySettings.load();
    expect(s2.contentWidth, 800);
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

  testWidgets('Chat tab has no Stats dropdown; slider updates width', (
    tester,
  ) async {
    final s = await ChatDisplaySettings.load();
    await tester.pumpWidget(MaterialApp(home: ChatTab(settings: s)));
    expect(find.byKey(const Key('stats-visibility')), findsNothing);
    expect(find.byKey(const Key('content-width')), findsOneWidget);
    await tester.drag(
      find.byKey(const Key('content-width')),
      const Offset(40, 0),
    );
    await tester.pumpAndSettle();
    expect(s.contentWidth, isNot(720));
  });

  testWidgets('hidden thinking still shows answer and caption', (tester) async {
    final appearance = await AppearanceSettings.load();
    await tester.pumpWidget(
      MaterialApp(
        theme: appearance.lightTheme,
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
