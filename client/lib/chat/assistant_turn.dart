import 'package:flutter/material.dart';

import 'chat_message.dart';
import 'display_settings.dart';

class AssistantTurnTile extends StatelessWidget {
  const AssistantTurnTile({
    super.key,
    required this.message,
    this.thinkingMode = VisibilityMode.collapsed,
    this.statsMode = VisibilityMode.collapsed,
  });

  final ChatMessage message;
  final VisibilityMode thinkingMode;
  final VisibilityMode statsMode;

  @override
  Widget build(BuildContext context) {
    final thought = message.thought;
    final caption = _caption(message);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(message.text.isEmpty ? '…' : message.text),
        if (caption.isNotEmpty) Text(caption),
        if (thought != null &&
            thought.isNotEmpty &&
            thinkingMode != VisibilityMode.hidden)
          ExpansionTile(
            key: Key(
              'thinking-${message.streamingThought}-${thought.hashCode}',
            ),
            title: const Text('Thinking'),
            initiallyExpanded:
                message.streamingThought ||
                thinkingMode == VisibilityMode.expanded,
            children: [Text(thought)],
          ),
        if (statsMode != VisibilityMode.hidden)
          ExpansionTile(
            title: const Text('Stats'),
            initiallyExpanded: statsMode == VisibilityMode.expanded,
            children: [for (final line in _statsLines(message)) Text(line)],
          ),
      ],
    );
  }
}

String _caption(ChatMessage message) {
  final tok = message.usage?.predictedPerSecond;
  return [
    message.model,
    message.providerName,
    if (tok != null) '$tok tok/s',
  ].whereType<String>().where((part) => part.isNotEmpty).join(' · ');
}

List<String> _statsLines(ChatMessage message) {
  final usage = message.usage;
  final stop = message.stopReason ?? usage?.stopReason;
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
