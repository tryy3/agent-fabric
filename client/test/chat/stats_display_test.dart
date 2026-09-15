import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/stats_display.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('normalizedStatRows uses pretty labels for known fields', () {
    final rows = normalizedStatRows(
      ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: const TurnUsage(
          promptTokens: 10,
          elapsedMs: 50,
          predictedPerSecond: 12.5,
        ),
        stopReason: 'end_turn',
      ),
    );
    expect(
      rows.map((r) => r.label).toList(),
      containsAll([
        'Prompt tokens',
        'Elapsed time',
        'Generation tokens / sec',
        'Stop reason',
      ]),
    );
    expect(rows.any((r) => r.key == 'promptTokens' && r.value == 10), isTrue);
  });

  test('unknown extras appear normalized and in raw JSON', () {
    final bubble = ChatBubble(
      kind: ChatBubbleKind.stats,
      usage: const TurnUsage(
        elapsedMs: 1,
        extras: {'cacheHitRatio': 0.42, 'vendorMetric': 'x'},
      ),
    );
    final rows = normalizedStatRows(bubble);
    expect(rows.any((r) => r.key == 'cacheHitRatio' && !r.known), isTrue);
    expect(rows.any((r) => r.label == 'Cache hit ratio'), isTrue);
    final raw = rawStatsJson(bubble);
    expect(raw, contains('"cacheHitRatio": 0.42'));
    expect(raw, contains('"vendorMetric": "x"'));
    expect(raw, contains('"elapsedMs": 1'));
  });

  test('humanizeStatKey splits camelCase', () {
    expect(humanizeStatKey('promptTokens'), 'Prompt tokens');
    expect(humanizeStatKey('ttftMs'), 'Ttft ms');
    expect(humanizeStatKey('cacheHitRatio'), 'Cache hit ratio');
  });
}
