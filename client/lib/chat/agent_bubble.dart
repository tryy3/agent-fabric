import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';
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
      ChatBubbleKind.thought => thinkingMode == VisibilityMode.hidden
          ? const SizedBox.shrink()
          : Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: _ThoughtActivity(
                key: ValueKey(
                  'thinking-${bubble.streamingThought}-$thinkingMode',
                ),
                bubble: bubble,
                thinkingMode: thinkingMode,
              ),
            ),
      ChatBubbleKind.message => Padding(
          padding: const EdgeInsets.symmetric(vertical: 8),
          child: _MessageProse(bubble: bubble, stats: stats),
        ),
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
    final chat = theme.extension<ChatColors>()!;
    final muted = theme.colorScheme.onSurfaceVariant;
    final body = widget.bubble.text;
    final header = InkWell(
      key: const Key('activity-thinking'),
      onTap: () => setState(() => _expanded = !_expanded),
      borderRadius: BorderRadius.circular(8),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
        child: Row(
          children: [
            Icon(
              Icons.lightbulb_outline,
              size: 18,
              color: chat.thinking.bar,
            ),
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
              color: muted,
            ),
          ],
        ),
      ),
    );

    if (!_expanded) {
      return header;
    }

    return Container(
      width: double.infinity,
      decoration: BoxDecoration(
        color: chat.thinking.fill,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          header,
          if (body.isNotEmpty)
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
              child: Text(body),
            ),
        ],
      ),
    );
  }
}

class _MessageProse extends StatelessWidget {
  const _MessageProse({required this.bubble, required this.stats});

  final ChatBubble bubble;
  final ChatBubble? stats;

  @override
  Widget build(BuildContext context) {
    final caption = bubbleCaption(bubble);
    final chat = Theme.of(context).extension<ChatColors>()!;
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(bubble.text.isEmpty ? '…' : bubble.text),
        if (caption.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              caption,
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: muted,
              ),
            ),
          ),
        if (_hasStats(stats)) ...[
          const SizedBox(height: 8),
          Material(
            color: chat.stats.fill,
            borderRadius: BorderRadius.circular(8),
            child: InkWell(
              key: const Key('stats-action'),
              borderRadius: BorderRadius.circular(8),
              onTap: () => showDialog<void>(
                context: context,
                builder: (context) => AlertDialog(
                  title: const Text('Stats'),
                  content: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      for (final line in statsLines(stats!)) Text(line),
                    ],
                  ),
                  actions: [
                    TextButton(
                      onPressed: () => Navigator.pop(context),
                      child: const Text('Close'),
                    ),
                  ],
                ),
              ),
              child: Padding(
                padding: const EdgeInsets.symmetric(
                  horizontal: 10,
                  vertical: 8,
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.bar_chart, size: 18, color: chat.stats.bar),
                    const SizedBox(width: 6),
                    Text(
                      'Stats',
                      style: TextStyle(
                        fontWeight: FontWeight.w600,
                        color: chat.stats.bar,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
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
