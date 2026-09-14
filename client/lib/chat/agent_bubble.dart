import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';
import 'chat_bubble.dart';
import 'display_settings.dart';

class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    this.thinkingMode = VisibilityMode.collapsed,
    this.statsMode = VisibilityMode.collapsed,
  });

  final ChatBubble bubble;
  final VisibilityMode thinkingMode;
  final VisibilityMode statsMode;

  @override
  Widget build(BuildContext context) {
    return switch (bubble.kind) {
      ChatBubbleKind.user => const SizedBox.shrink(),
      ChatBubbleKind.thought => _thought(context),
      ChatBubbleKind.message => _message(context),
      ChatBubbleKind.stats => _stats(context),
    };
  }

  Widget _thought(BuildContext context) {
    if (thinkingMode == VisibilityMode.hidden) {
      return const SizedBox.shrink();
    }
    final chat = Theme.of(context).extension<ChatColors>()!;
    return _card(
      bar: chat.thinking.bar,
      fill: chat.thinking.fill,
      child: ExpansionTile(
        key: Key('thinking-${bubble.streamingThought}-$thinkingMode'),
        title: const Text('Thinking'),
        tilePadding: EdgeInsets.zero,
        childrenPadding: EdgeInsets.zero,
        expandedAlignment: Alignment.centerLeft,
        expandedCrossAxisAlignment: CrossAxisAlignment.start,
        controlAffinity: ListTileControlAffinity.trailing,
        initiallyExpanded:
            bubble.streamingThought || thinkingMode == VisibilityMode.expanded,
        children: [
          Align(alignment: Alignment.centerLeft, child: Text(bubble.text)),
        ],
      ),
    );
  }

  Widget _message(BuildContext context) {
    final chat = Theme.of(context).extension<ChatColors>()!;
    final caption = bubbleCaption(bubble);
    return _card(
      bar: chat.answer.bar,
      fill: chat.answer.fill,
      child: Column(
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
        ],
      ),
    );
  }

  Widget _stats(BuildContext context) {
    final stop = bubble.stopReason ?? '';
    if (statsMode == VisibilityMode.hidden ||
        (bubble.usage == null && stop.isEmpty)) {
      return const SizedBox.shrink();
    }
    final chat = Theme.of(context).extension<ChatColors>()!;
    return _card(
      bar: chat.stats.bar,
      fill: chat.stats.fill,
      child: ExpansionTile(
        key: Key('stats-$statsMode'),
        title: const Text('Stats'),
        tilePadding: EdgeInsets.zero,
        childrenPadding: EdgeInsets.zero,
        expandedAlignment: Alignment.centerLeft,
        expandedCrossAxisAlignment: CrossAxisAlignment.start,
        controlAffinity: ListTileControlAffinity.trailing,
        initiallyExpanded: statsMode == VisibilityMode.expanded,
        children: [
          for (final line in _statsLines(bubble))
            Align(alignment: Alignment.centerLeft, child: Text(line)),
        ],
      ),
    );
  }

  Widget _card({
    required Color bar,
    required Color fill,
    required Widget child,
  }) {
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 4),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: fill,
          borderRadius: BorderRadius.circular(12),
          border: Border(left: BorderSide(color: bar, width: 4)),
        ),
        child: Material(color: fill, child: child),
      ),
    );
  }
}

List<String> _statsLines(ChatBubble bubble) {
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
