import '../acp/agent_connection.dart';

/// USD amount with enough precision for small LLM costs: "$0.0045", "$0.0432",
/// "$0.432", "$1.20".
String formatUsd(double usd) {
  if (usd == 0) return r'$0';
  if (usd < 0.1) return '\$${usd.toStringAsFixed(4)}';
  if (usd < 1) return '\$${usd.toStringAsFixed(3)}';
  return '\$${usd.toStringAsFixed(2)}';
}

/// Short cost label: "~$0.0045" for an estimate, with a trailing "+" when
/// some prices were unknown so the real cost is higher.
String formatCost(TurnCost cost) {
  final amount = formatUsd(cost.total);
  final marker = cost.estimated ? '~' : '';
  return '$marker$amount${cost.partial ? '+' : ''}';
}

/// Explains where the cost number comes from, for tooltips.
String costTooltip(TurnCost cost) {
  final lead = cost.estimated
      ? 'Estimated from the model\'s published prices; not exact billing.'
      : 'Cost.';
  return cost.partial
      ? '$lead Some prices were unknown, so this is a lower bound.'
      : lead;
}

/// One-line breakdown of non-zero components, e.g.
/// "input $0.003 · cached $0.0003 · output $0.0015".
String costBreakdownText(TurnCost cost) {
  final parts = <String>[
    if (cost.input > 0) 'input ${formatUsd(cost.input)}',
    if (cost.cacheRead > 0) 'cached ${formatUsd(cost.cacheRead)}',
    if (cost.cacheWrite > 0) 'cache write ${formatUsd(cost.cacheWrite)}',
    if (cost.output > 0) 'output ${formatUsd(cost.output)}',
    if (cost.reasoning > 0) 'reasoning ${formatUsd(cost.reasoning)}',
  ];
  return parts.join(' · ');
}
