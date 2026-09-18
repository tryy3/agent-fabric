import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
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

  test('defaults: contentWidth 720', () async {
    final s = await ChatDisplaySettings.load();
    expect(s.contentWidth, 720);
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

  testWidgets('Display tab has no thinking visibility control', (tester) async {
    await pumpDisplay(tester);
    expect(find.byKey(const Key('thinking-visibility')), findsNothing);
    expect(find.text('Collapsed'), findsNothing);
  });

  testWidgets('Display tab has preview and content-width slider', (
    tester,
  ) async {
    await pumpDisplay(tester);
    expect(find.byKey(const Key('stats-visibility')), findsNothing);
    expect(find.byKey(const Key('content-width')), findsOneWidget);
    expect(find.byKey(const Key('content-width-preview')), findsOneWidget);
  });

  testWidgets('preview column and band match contentWidth in pixels', (
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

    double widthOf(Key key) {
      return tester.renderObject<RenderBox>(find.byKey(key)).size.width;
    }

    expect(widthOf(const Key('content-width-column')), 720);
    expect(widthOf(const Key('content-width')), 720);

    await display.setContentWidth(1080);
    await tester.pumpAndSettle();
    expect(widthOf(const Key('content-width-column')), 1080);
    expect(widthOf(const Key('content-width')), 1080);
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
                viewMode: const ViewMode(
                  id: 'hidden-thinking',
                  label: 'Hidden thinking',
                  markdownRender: false,
                  thinkingVisibility: VisibilityMode.hidden,
                  toolVisibility: VisibilityMode.collapsed,
                  toolIO: ToolIOMode.both,
                ),
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: 'hmm',
                ),
              ),
              AgentBubble(
                viewMode: resolveViewMode('detailed'),
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
