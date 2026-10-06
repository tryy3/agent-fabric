import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import 'cost_dialog.dart';
import 'cost_format.dart';

/// Header badge with the thread's total cost; tap for the breakdown.
class ThreadTotalsLabel extends StatelessWidget {
  const ThreadTotalsLabel({super.key, required this.totals});

  final ThreadTotals totals;

  @override
  Widget build(BuildContext context) {
    final cost = totals.cost;
    final text = cost != null
        ? formatCost(cost)
        : totals.reportedCostUsd != null
        ? '${formatUsd(totals.reportedCostUsd!)} (reported)'
        : 'totals';
    final color = Theme.of(context).colorScheme.primary;
    return Tooltip(
      message: 'Total cost of this thread. Tap for details.',
      child: Material(
        color: color.withValues(alpha: 0.14),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(999),
          side: BorderSide(color: color.withValues(alpha: 0.5)),
        ),
        child: InkWell(
          key: const Key('thread-totals'),
          customBorder: const StadiumBorder(),
          onTap: () => showDialog<void>(
            context: context,
            builder: (_) => ThreadTotalsDialog(totals: totals),
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 3),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.receipt_long_outlined, size: 14, color: color),
                const SizedBox(width: 5),
                Text(
                  'Thread $text',
                  style: Theme.of(context).textTheme.labelMedium
                      ?.copyWith(color: color, fontWeight: FontWeight.w600),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class ThreadTotalsDialog extends StatelessWidget {
  const ThreadTotalsDialog({super.key, required this.totals});

  final ThreadTotals totals;

  @override
  Widget build(BuildContext context) {
    final cost = totals.cost;
    return CostDetailsDialog(
      title: 'Thread totals',
      rows: [
        ('Turns', '${totals.turns}'),
        ('LLM requests', '${totals.requests}'),
        ('Prompt tokens', '${totals.promptTokens}'),
        ('Completion tokens', '${totals.completionTokens}'),
        if (totals.cachedTokens > 0)
          ('Cached tokens', '${totals.cachedTokens}'),
        if (totals.cacheWriteTokens > 0)
          ('Cache write tokens', '${totals.cacheWriteTokens}'),
        if (totals.reasoningTokens > 0)
          ('Reasoning tokens', '${totals.reasoningTokens}'),
        if (cost != null) ...[
          ('Estimated cost', formatCost(cost)),
          if (costBreakdownText(cost).isNotEmpty)
            ('Breakdown', costBreakdownText(cost)),
        ],
        if (totals.reportedCostUsd != null)
          ('Provider-reported cost', formatUsd(totals.reportedCostUsd!)),
      ],
      footnote:
          'Every LLM call of every attempt in this thread, retried and failed '
          'ones included. Estimates use the model\'s published prices; not '
          'exact billing.',
    );
  }
}
