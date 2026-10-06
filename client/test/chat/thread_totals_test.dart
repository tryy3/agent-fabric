import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('ThreadTotals reads the plane totals', () {
    final totals = ThreadTotals.fromJson({
      'turns': 2,
      'requests': 3,
      'promptTokens': 150,
      'completionTokens': 15,
      'cachedTokens': 20,
      'cost': {'total': 0.003, 'partial': true},
      'reportedCostUsd': 0.004,
    });
    expect(totals.turns, 2);
    expect(totals.requests, 3);
    expect(totals.promptTokens, 150);
    expect(totals.cachedTokens, 20);
    expect(totals.cost!.total, closeTo(0.003, 1e-9));
    expect(totals.cost!.partial, isTrue);
    expect(totals.reportedCostUsd, 0.004);
    expect(const ThreadTotals().isEmpty, isTrue);
    expect(ThreadTotals.fromJson(null).isEmpty, isTrue);
  });

  test('ThreadTotals adds the running cost of a streaming turn', () {
    final totals = ThreadTotals.fromJson({
      'requests': 1,
      'cost': {'total': 0.001},
    }).withRunning(const TurnCost(total: 0.0005));
    expect(totals.cost!.total, closeTo(0.0015, 1e-9));
    expect(
      const ThreadTotals().withRunning(const TurnCost(total: 1)).cost,
      isNotNull,
    );
    expect(totals.withRunning(null), same(totals));
  });
}
