import 'package:flutter/material.dart';

import 'chat_bubble.dart';
import 'display_settings.dart';

class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    this.thinkingMode = VisibilityMode.collapsed,
  });

  final ChatBubble bubble;
  final VisibilityMode thinkingMode;

  @override
  Widget build(BuildContext context) {
    return switch (bubble.kind) {
      ChatBubbleKind.user => const SizedBox.shrink(),
      ChatBubbleKind.thought => _thought(),
      ChatBubbleKind.message => _message(context),
      ChatBubbleKind.stats => _stats(),
    };
  }

  Widget _thought() {
    if (thinkingMode == VisibilityMode.hidden) {
      return const SizedBox.shrink();
    }
    return _card(
      bar: const Color(0xFFD97706),
      fill: const Color(0xFFFEF3C7),
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
    final caption = bubbleCaption(bubble);
    return _card(
      bar: const Color(0xFF0F766E),
      fill: const Color(0xFFCCFBF1),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(bubble.text.isEmpty ? '…' : bubble.text),
          if (caption.isNotEmpty)
            Text(
              caption,
              style: Theme.of(context).textTheme.bodySmall
                  ?.copyWith(color: const Color(0xFF71717A)),
            ),
        ],
      ),
    );
  }

  Widget _stats() {
    final stop = bubble.stopReason ?? '';
    if (bubble.usage == null && stop.isEmpty) {
      return const SizedBox.shrink();
    }
    return _card(
      bar: const Color(0xFF71717A),
      fill: const Color(0xFFE4E4E7),
      child: ExpansionTile(
        key: const Key('stats-collapsed'),
        title: const Text('Stats'),
        tilePadding: EdgeInsets.zero,
        childrenPadding: EdgeInsets.zero,
        expandedAlignment: Alignment.centerLeft,
        expandedCrossAxisAlignment: CrossAxisAlignment.start,
        controlAffinity: ListTileControlAffinity.trailing,
        initiallyExpanded: false,
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
