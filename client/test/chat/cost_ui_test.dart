import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/cost_format.dart';
import 'package:agent_fabric_client/chat/stats_display.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
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
