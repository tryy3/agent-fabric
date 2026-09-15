import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/display_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
  });

  Future<DisplayTab> pumpDisplay(WidgetTester tester) async {
    final display = await ChatDisplaySettings.load();
    final appearance = await AppearanceSettings.load();
    final tab = DisplayTab(
      displaySettings: display,
      appearanceSettings: appearance,
    );
    await tester.pumpWidget(MaterialApp(home: tab));
    return tab;
  }

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
    await s.setContentWidth(500);
    expect(s.contentWidth, 560);
    await s.setContentWidth(2000);
    expect(s.contentWidth, 1200);
    await s.setContentWidth(800);
    final s2 = await ChatDisplaySettings.load();
    expect(s2.contentWidth, 800);
  });

  testWidgets('Display tab updates thinking visibility', (tester) async {
    final tab = await pumpDisplay(tester);
    expect(find.text('Thinking'), findsWidgets);
    await tester.tap(find.byKey(const Key('thinking-visibility')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Hidden').last);
    await tester.pumpAndSettle();
    expect(tab.displaySettings.thinking, VisibilityMode.hidden);
  });

  testWidgets('Display tab has preview and content-width slider', (
    tester,
  ) async {
    await pumpDisplay(tester);
    expect(find.byKey(const Key('stats-visibility')), findsNothing);
    expect(find.byKey(const Key('content-width')), findsOneWidget);
    expect(find.byKey(const Key('content-width-preview')), findsOneWidget);
  });

  testWidgets('content width preview column grows with setting', (
    tester,
  ) async {
    final display = await ChatDisplaySettings.load();
    final appearance = await AppearanceSettings.load();
    await tester.pumpWidget(
      MaterialApp(
        home: SizedBox(
          width: 400,
          child: DisplayTab(
            displaySettings: display,
            appearanceSettings: appearance,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    double columnWidth() {
      final box = tester.renderObject<RenderBox>(
        find.byKey(const Key('content-width-column')),
      );
      return box.size.width;
    }

    final narrow = columnWidth();
    await display.setContentWidth(1200);
    await tester.pumpAndSettle();
    expect(columnWidth(), greaterThan(narrow));
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
  });
}
