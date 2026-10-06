import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/cost_dialog.dart';
import 'package:agent_fabric_client/chat/cost_format.dart';
import 'package:agent_fabric_client/chat/stats_display.dart';
import 'package:agent_fabric_client/chat/thread_totals.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  _roundAndThreadTests();
  _roundCaptureTests();
  _turnCaptureTests();
  test('formatUsd keeps precision for small amounts', () {
    expect(formatUsd(0), r'$0');
    expect(formatUsd(0.0045), r'$0.0045');
    expect(formatUsd(0.0432), r'$0.0432');
    expect(formatUsd(0.432), r'$0.432');
    expect(formatUsd(1.2), r'$1.20');
  });

  test('formatCost marks estimates and lower bounds', () {
    expect(formatCost(const TurnCost(total: 0.0108)), r'~$0.0108');
    expect(
      formatCost(const TurnCost(total: 0.0108, partial: true)),
      r'~$0.0108+',
    );
    expect(formatCost(const TurnCost(total: 2, estimated: false)), r'$2.00');
    expect(
      costBreakdownText(
        const TurnCost(total: 0.0045, input: 0.003, output: 0.0015),
      ),
      r'input $0.0030 · output $0.0015',
    );
  });

  test('stats rows show cost, token breakdown and per-round lines', () {
    final bubble = ChatBubble(
      kind: ChatBubbleKind.stats,
      usage: const TurnUsage(
        promptTokens: 3000,
        cachedTokens: 1000,
        reasoningTokens: 5,
        cost: TurnCost(total: 0.0108, input: 0.006, output: 0.0045),
        reportedCostUsd: 0.02,
        rounds: [
          {
            'round': 0,
            'promptTokens': 1000,
            'completionTokens': 100,
            'cost': {'total': 0.0045},
          },
          {'round': 1, 'promptTokens': 2000},
        ],
      ),
    );
    final rows = normalizedStatRows(bubble);
    String valueOf(String key) =>
        rows.firstWhere((r) => r.key == key).value as String;
    expect(valueOf('cost'), startsWith(r'~$0.0108 (input'));
    expect(valueOf('reportedCostUsd'), r'$0.0200');
    expect(rows.any((r) => r.key == 'cachedTokens' && r.value == 1000), isTrue);
    expect(valueOf('round0'), r'1000 in · 100 out · ~$0.0045');
    expect(valueOf('round1'), '2000 in');
    expect(rows.firstWhere((r) => r.key == 'round0').label, 'Round 1');

    final raw = rawStatsJson(bubble);
    expect(raw, contains('"cost"'));
    expect(raw, contains('"rounds"'));
    expect(raw, contains('"reportedCostUsd": 0.02'));
  });

  test('no price means no cost row', () {
    final rows = normalizedStatRows(
      ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: const TurnUsage(promptTokens: 5),
      ),
    );
    expect(
      rows.any((r) => r.key == 'cost' || r.key == 'reportedCostUsd'),
      isFalse,
    );
  });

  testWidgets('stats button shows the estimated cost when known', (
    tester,
  ) async {
    Future<void> pumpWith(TurnUsage usage) {
      return tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: Scaffold(
            body: AgentBubble(
              viewMode: resolveViewMode('detailed'),
              bubble: const ChatBubble(
                kind: ChatBubbleKind.message,
                text: 'hi',
              ),
              stats: ChatBubble(kind: ChatBubbleKind.stats, usage: usage),
            ),
          ),
        ),
      );
    }

    await pumpWith(
      const TurnUsage(elapsedMs: 5, cost: TurnCost(total: 0.0108)),
    );
    expect(find.byKey(const Key('stats-cost')), findsOneWidget);
    expect(find.text(r'~$0.0108'), findsOneWidget);

    await pumpWith(const TurnUsage(elapsedMs: 5));
    expect(find.byKey(const Key('stats-cost')), findsNothing);
  });
}

void _roundAndThreadTests() {
  testWidgets('tapping a round divider shows that round\'s details', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.roundCost,
              usage: TurnUsage(
                isPartial: true,
                round: 1,
                promptTokens: 2500,
                completionTokens: 23,
                reasoningTokens: 7,
                roundCost: TurnCost(total: 0.0007, input: 0.0006),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('round-cost')));
    await tester.pumpAndSettle();
    expect(find.text('Round 2'), findsOneWidget);
    expect(find.text('2500'), findsOneWidget);
    expect(find.text('Reasoning tokens'), findsOneWidget);
    expect(find.text('Estimated cost'), findsOneWidget);
    await tester.tap(find.byKey(const Key('stats-tab-raw')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('stats-raw-json')), findsOneWidget);
    expect(find.textContaining('"promptTokens": 2500'), findsOneWidget);
  });

  testWidgets('thread total badge opens the thread totals', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: ThreadTotalsLabel(
            totals: ThreadTotals.fromJson({
              'turns': 1,
              'requests': 1,
              'promptTokens': 10,
              'cost': {'total': 0.0123},
            }),
          ),
        ),
      ),
    );
    expect(find.textContaining('Thread ~'), findsOneWidget);
    await tester.tap(find.byKey(const Key('thread-totals')));
    await tester.pumpAndSettle();
    expect(find.text('Thread totals'), findsOneWidget);
    expect(find.text('Turns'), findsOneWidget);
  });
}

class _CaptureCatalog extends CatalogClient {
  _CaptureCatalog(this.captures)
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('[]', 200)),
      );

  final List<HopCapture> captures;

  @override
  Future<List<HopCapture>> listMessageCaptures(
    String threadId,
    String messageId,
  ) async => captures;
}

void _roundCaptureTests() {
  testWidgets('a stored round shows the captured usage and request', (
    tester,
  ) async {
    HopCapture capture(int round, int promptMs) => HopCapture(
      id: 'cap_$round',
      threadId: 'th_1',
      messageId: 'm_1',
      roundIndex: round,
      hopKind: 'llm',
      direction: 'exchange',
      headers: const {},
      bodyText: '{"model":"m1"}',
      meta: {
        'model': 'm1',
        'usage': {'promptTokens': 99, 'promptMs': promptMs, 'ttftMs': 12},
      },
      createdAt: DateTime.utc(2026),
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () => showStatsDialog(
                context,
                const ChatBubble(
                  kind: ChatBubbleKind.roundCost,
                  catalogMessageId: 'm_1',
                  usage: TurnUsage(
                    isPartial: true,
                    round: 1,
                    promptTokens: 5,
                    roundCost: TurnCost(total: 0.0007),
                  ),
                ),
                catalog: _CaptureCatalog([capture(0, 1), capture(1, 77)]),
                threadId: 'th_1',
              ),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    // Normalized comes from the round's capture, cost from the plane.
    expect(find.text('99'), findsOneWidget);
    expect(find.text('Prompt eval time'), findsOneWidget);
    expect(find.text('Estimated cost'), findsOneWidget);
    // Raw is the captured request, not a stats dump.
    await tester.tap(find.byKey(const Key('stats-tab-raw')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('stats-raw-view')), findsOneWidget);
  });
}

void _turnCaptureTests() {
  testWidgets('a turn\'s stats open the same dialog, Raw lists every round', (
    tester,
  ) async {
    HopCapture capture(int round) => HopCapture(
      id: 'cap_$round',
      threadId: 'th_1',
      messageId: 'm_1',
      roundIndex: round,
      hopKind: 'llm',
      direction: 'exchange',
      headers: const {},
      bodyText: '{"round":$round}',
      meta: const {},
      createdAt: DateTime.utc(2026),
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () => showStatsDialog(
                context,
                const ChatBubble(
                  kind: ChatBubbleKind.stats,
                  catalogMessageId: 'm_1',
                  usage: TurnUsage(promptTokens: 7),
                ),
                catalog: _CaptureCatalog([capture(0), capture(1)]),
                threadId: 'th_1',
              ),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    final narrow = tester.getSize(find.byType(TabBar)).width;
    await tester.tap(find.byKey(const Key('stats-tab-raw')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('stats-raw-round-0')), findsOneWidget);
    expect(find.byKey(const Key('stats-raw-round-1')), findsOneWidget);
    // The dialog widens for the request inspector.
    expect(tester.getSize(find.byType(TabBar)).width, greaterThan(narrow));
  });
}
