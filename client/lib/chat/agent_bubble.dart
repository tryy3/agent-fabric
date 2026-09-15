import 'package:flutter/material.dart';

import 'chat_bubble.dart';
import 'display_settings.dart';

class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    this.thinkingMode = VisibilityMode.collapsed,
    this.stats,
  });

  final ChatBubble bubble;
  final VisibilityMode thinkingMode;
  final ChatBubble? stats;

  @override
  Widget build(BuildContext context) {
    return switch (bubble.kind) {
      ChatBubbleKind.user => const SizedBox.shrink(),
      ChatBubbleKind.thought =>
        thinkingMode == VisibilityMode.hidden
            ? const SizedBox.shrink()
            : _ThoughtActivity(
                key: ValueKey(
                  'thinking-${bubble.streamingThought}-$thinkingMode',
                ),
                bubble: bubble,
                thinkingMode: thinkingMode,
              ),
      ChatBubbleKind.message => _MessageProse(bubble: bubble, stats: stats),
      ChatBubbleKind.stats => const SizedBox.shrink(),
    };
  }
}

class _ThoughtActivity extends StatefulWidget {
  const _ThoughtActivity({
    super.key,
    required this.bubble,
    required this.thinkingMode,
  });

  final ChatBubble bubble;
  final VisibilityMode thinkingMode;

  @override
  State<_ThoughtActivity> createState() => _ThoughtActivityState();
}

class _ThoughtActivityState extends State<_ThoughtActivity> {
  late bool _expanded =
      widget.bubble.streamingThought ||
      widget.thinkingMode == VisibilityMode.expanded;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final muted = theme.colorScheme.onSurfaceVariant;
    final details = _activityBodyLines(widget.bubble.text);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        InkWell(
          key: const Key('activity-thinking'),
          onTap: () => setState(() => _expanded = !_expanded),
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 8),
            child: Row(
              children: [
                const Icon(Icons.lightbulb_outline, size: 18),
                const SizedBox(width: 8),
                const Text(
                  'Thinking',
                  style: TextStyle(fontWeight: FontWeight.bold),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    activityDescription(widget.bubble.text),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: theme.textTheme.bodySmall?.copyWith(color: muted),
                  ),
                ),
                Icon(
                  _expanded ? Icons.expand_less : Icons.expand_more,
                  size: 20,
                ),
              ],
            ),
          ),
        ),
        if (_expanded && details.isNotEmpty)
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            color: theme.colorScheme.surfaceContainerHighest,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [for (final line in details) Text(line)],
            ),
          ),
      ],
    );
  }
}

List<String> _activityBodyLines(String text) {
  final lines = text.split('\n');
  final descriptionIndex = lines.indexWhere((line) => line.trim().isNotEmpty);
  if (descriptionIndex < 0 || descriptionIndex == lines.length - 1) {
    return const [];
  }
  return lines.sublist(descriptionIndex + 1);
}

class _MessageProse extends StatelessWidget {
  const _MessageProse({required this.bubble, required this.stats});

  final ChatBubble bubble;
  final ChatBubble? stats;

  @override
  Widget build(BuildContext context) {
    final caption = bubbleCaption(bubble);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(bubble.text.isEmpty ? '…' : bubble.text),
        if (caption.isNotEmpty)
          Text(
            caption,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
          ),
        if (_hasStats(stats))
          TextButton(
            key: const Key('stats-action'),
            onPressed: () => showDialog<void>(
              context: context,
              builder: (context) => AlertDialog(
                title: const Text('Stats'),
                content: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [for (final line in statsLines(stats!)) Text(line)],
                ),
                actions: [
                  TextButton(
                    onPressed: () => Navigator.pop(context),
                    child: const Text('Close'),
                  ),
                ],
              ),
            ),
            child: const Text('Stats'),
          ),
      ],
    );
  }
}

bool _hasStats(ChatBubble? stats) {
  return stats != null &&
      (stats.usage != null || (stats.stopReason?.isNotEmpty ?? false));
}

List<String> statsLines(ChatBubble bubble) {
  final usage = bubble.usage;
  final stop = bubble.stopReason ?? usage?.stopReason;
  return [
    if (usage?.promptTokens != null) 'promptTokens: ${usage!.promptTokens}',
    if (usage?.completionTokens != null)
      'completionTokens: ${usage!.completionTokens}',
    if (usage?.totalTokens != null) 'totalTokens: ${usage!.totalTokens}',
    if (usage?.ttftMs != null) 'ttftMs: ${usage!.ttftMs}',
    if (usage?.elapsedMs != null) 'elapsedMs: ${usage!.elapsedMs}',
    if (usage?.promptMs != null) 'promptMs: ${usage!.promptMs}',
    if (usage?.predictedMs != null) 'predictedMs: ${usage!.predictedMs}',
    if (usage?.promptPerSecond != null)
      'promptPerSecond: ${usage!.promptPerSecond}',
    if (usage?.predictedPerSecond != null)
      'predictedPerSecond: ${usage!.predictedPerSecond}',
    if (usage?.deltas != null) 'deltas: ${usage!.deltas}',
    if (stop != null && stop.isNotEmpty) 'stopReason: $stop',
  ];
}
