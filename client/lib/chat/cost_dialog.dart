import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../acp/agent_connection.dart';
import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import 'chat_bubble.dart';
import 'cost_format.dart';
import 'inspector_http_view.dart';
import 'stats_display.dart';

/// A titled list of label/value rows with a footnote, used for thread and
/// round cost details.
class CostDetailsDialog extends StatelessWidget {
  const CostDetailsDialog({
    super.key,
    required this.title,
    required this.rows,
    required this.footnote,
  });

  final String title;
  final List<(String, String)> rows;
  final String footnote;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(title),
      content: SizedBox(
        width: 320,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (final (label, value) in rows)
                ListTile(
                  dense: true,
                  visualDensity: VisualDensity.compact,
                  contentPadding: EdgeInsets.zero,
                  title: Text(
                    label,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  subtitle: SelectableText(value),
                ),
              const SizedBox(height: 8),
              Text(footnote, style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Close'),
        ),
      ],
    );
  }
}

/// Opens the Stats-style dialog for one LLM round (a divider's bubble).
Future<void> showRoundStatsDialog(
  BuildContext context, {
  required CatalogClient? catalog,
  required String? threadId,
  required ChatBubble bubble,
}) {
  return showDialog<void>(
    context: context,
    builder: (_) => RoundStatsDialog(
      catalog: catalog,
      threadId: threadId,
      messageId: bubble.catalogMessageId,
      usage: bubble.usage!,
    ),
  );
}

/// One LLM round shown like a turn's Stats dialog. The divider already knows
/// the round's tokens and cost; a stored round also loads the request's hop
/// capture, which holds the normalized usage beside the raw (scrubbed) request
/// and response shown on the Raw tab.
class RoundStatsDialog extends StatefulWidget {
  const RoundStatsDialog({
    super.key,
    required this.catalog,
    required this.threadId,
    required this.messageId,
    required this.usage,
  });

  final CatalogClient? catalog;
  final String? threadId;
  final String? messageId;

  /// The round as the divider knows it.
  final TurnUsage usage;

  @override
  State<RoundStatsDialog> createState() => _RoundStatsDialogState();
}

class _RoundStatsDialogState extends State<RoundStatsDialog> {
  HopCapture? _capture;

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  Future<void> _load() async {
    final catalog = widget.catalog;
    final threadId = widget.threadId;
    final messageId = widget.messageId;
    if (catalog == null || threadId == null || messageId == null) return;
    try {
      final captures = await catalog.listMessageCaptures(threadId, messageId);
      final match = captures.where(
        (c) => c.hopKind == 'llm' && c.roundIndex == widget.usage.round,
      );
      if (match.isNotEmpty && mounted) {
        setState(() => _capture = match.first);
      }
    } on Object catch (_) {
      // The divider's own data still shows.
    }
  }

  @override
  Widget build(BuildContext context) {
    final lean = widget.usage;
    var usage = lean.asRound();
    final stored = _capture?.meta['usage'];
    if (stored is Map) {
      final model = _capture?.meta['model'];
      usage = turnUsageFromMeta({
        ...stored.cast<String, Object?>(),
        'model': ?(model is String ? model : lean.model),
        'round': lean.round,
      }).withCost(cost: lean.roundCost);
    }
    final capture = _capture;
    return StatsDialog(
      title: 'Round ${(lean.round ?? 0) + 1}',
      stats: ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: usage,
        stopReason: usage.stopReason,
      ),
      rawView: capture == null ? null : InspectorHttpView(capture: capture),
    );
  }
}
