import 'dart:convert';

import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';
import 'chat_bubble.dart';
import 'display_settings.dart';
import 'message_text.dart';
import 'stats_display.dart';
import 'view_modes.dart';

class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    required this.viewMode,
    this.stats,
  });

  final ChatBubble bubble;
  final ViewMode viewMode;
  final ChatBubble? stats;

  @override
  Widget build(BuildContext context) {
    final thinkingVisibility = viewMode.thinkingVisibility;
    return switch (bubble.kind) {
      ChatBubbleKind.user => const SizedBox.shrink(),
      ChatBubbleKind.thought =>
        thinkingVisibility == VisibilityMode.hidden
            ? const SizedBox.shrink()
            : Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: _ThoughtActivity(
                  key: ValueKey(
                    'thinking-${bubble.streamingThought}-$thinkingVisibility',
                  ),
                  bubble: bubble,
                  thinkingVisibility: thinkingVisibility,
                ),
              ),
      ChatBubbleKind.toolCall =>
        viewMode.toolVisibility == VisibilityMode.hidden
            ? const SizedBox.shrink()
            : Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: _ToolCallActivity(
                  key: ValueKey(
                    'tool-${bubble.toolCallId}-${bubble.streamingTool}',
                  ),
                  bubble: bubble,
                  toolVisibility: viewMode.toolVisibility,
                  toolIO: viewMode.toolIO,
                ),
              ),
      ChatBubbleKind.message => Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: _MessageProse(
          bubble: bubble,
          stats: stats,
          markdown: viewMode.markdownRender,
        ),
      ),
      ChatBubbleKind.stats => const SizedBox.shrink(),
    };
  }
}

class _ToolCallActivity extends StatefulWidget {
  const _ToolCallActivity({
    super.key,
    required this.bubble,
    required this.toolVisibility,
    required this.toolIO,
  });

  final ChatBubble bubble;
  final VisibilityMode toolVisibility;
  final ToolIOMode toolIO;

  @override
  State<_ToolCallActivity> createState() => _ToolCallActivityState();
}

class _ToolCallActivityState extends State<_ToolCallActivity> {
  late bool _expanded =
      widget.bubble.streamingTool ||
      widget.toolVisibility == VisibilityMode.expanded;
  late int _tabIndex = _defaultTabIndex(widget.bubble, widget.toolIO);

  static int _defaultTabIndex(ChatBubble bubble, ToolIOMode toolIO) {
    if (toolIO == ToolIOMode.input) {
      return 0;
    }
    if (toolIO == ToolIOMode.output) {
      return 1;
    }
    final output = _formatToolValue(bubble.toolOutput);
    return output.isNotEmpty ? 1 : 0;
  }

  @override
  void didUpdateWidget(covariant _ToolCallActivity oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.toolIO != ToolIOMode.both) {
      return;
    }
    final hadOutput = _formatToolValue(oldWidget.bubble.toolOutput).isNotEmpty;
    final hasOutput = _formatToolValue(widget.bubble.toolOutput).isNotEmpty;
    if (!hadOutput && hasOutput && _tabIndex == 0) {
      _tabIndex = 1;
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final chat = theme.extension<ChatColors>()!;
    final muted = theme.colorScheme.onSurfaceVariant;
    final input = _formatToolValue(widget.bubble.toolInput);
    final output = _formatToolValue(widget.bubble.toolOutput);
    final header = InkWell(
      key: Key('activity-tool-${widget.bubble.toolCallId}'),
      onTap: () => setState(() => _expanded = !_expanded),
      borderRadius: BorderRadius.circular(8),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
        child: Row(
          children: [
            Icon(Icons.build_outlined, size: 18, color: chat.thinking.bar),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                widget.bubble.toolTitle ?? 'Tool call',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontWeight: FontWeight.bold),
              ),
            ),
            if (widget.bubble.toolStatus case final status?)
              Text(
                status.replaceAll('_', ' '),
                style: theme.textTheme.bodySmall?.copyWith(color: muted),
              ),
            const SizedBox(width: 8),
            Icon(
              _expanded ? Icons.expand_less : Icons.expand_more,
              size: 20,
              color: muted,
            ),
          ],
        ),
      ),
    );
    if (!_expanded) return header;

    final body = switch (widget.toolIO) {
      ToolIOMode.input => input,
      ToolIOMode.output => output,
      ToolIOMode.both => _tabIndex == 0 ? input : output,
    };

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
          if (widget.toolIO == ToolIOMode.both)
            Padding(
              padding: const EdgeInsets.fromLTRB(10, 0, 10, 0),
              child: Row(
                children: [
                  _ToolTab(
                    label: 'Input',
                    selected: _tabIndex == 0,
                    onTap: () => setState(() => _tabIndex = 0),
                  ),
                  const SizedBox(width: 8),
                  _ToolTab(
                    label: 'Output',
                    selected: _tabIndex == 1,
                    onTap: () => setState(() => _tabIndex = 1),
                  ),
                ],
              ),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
            child: SelectableText(
              body.isEmpty ? '—' : body,
              style: const TextStyle(fontFamily: 'monospace'),
            ),
          ),
        ],
      ),
    );
  }
}

class _ToolTab extends StatelessWidget {
  const _ToolTab({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final muted = theme.colorScheme.onSurfaceVariant;
    return InkWell(
      key: Key('tool-tab-${label.toLowerCase()}'),
      onTap: onTap,
      borderRadius: BorderRadius.circular(6),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        child: Text(
          label,
          style: theme.textTheme.labelLarge?.copyWith(
            fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
            color: selected ? theme.colorScheme.onSurface : muted,
          ),
        ),
      ),
    );
  }
}

String _formatToolValue(Object? value) {
  if (value == null) return '';
  Object? decoded = value;
  if (value is String) {
    try {
      decoded = jsonDecode(value);
    } on FormatException {
      return value;
    }
  }
  if (decoded is Map || decoded is List) {
    return const JsonEncoder.withIndent('  ').convert(decoded);
  }
  return decoded.toString();
}

class _ThoughtActivity extends StatefulWidget {
  const _ThoughtActivity({
    super.key,
    required this.bubble,
    required this.thinkingVisibility,
  });

  final ChatBubble bubble;
  final VisibilityMode thinkingVisibility;

  @override
  State<_ThoughtActivity> createState() => _ThoughtActivityState();
}

class _ThoughtActivityState extends State<_ThoughtActivity> {
  late bool _expanded =
      widget.bubble.streamingThought ||
      widget.thinkingVisibility == VisibilityMode.expanded;

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
            Icon(Icons.lightbulb_outline, size: 18, color: chat.thinking.bar),
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
  const _MessageProse({
    required this.bubble,
    required this.stats,
    required this.markdown,
  });

  final ChatBubble bubble;
  final ChatBubble? stats;
  final bool markdown;

  @override
  Widget build(BuildContext context) {
    final caption = bubbleCaption(bubble);
    final chat = Theme.of(context).extension<ChatColors>()!;
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        MessageText(
          text: bubble.text.isEmpty ? '…' : bubble.text,
          markdown: markdown,
        ),
        if (caption.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              caption,
              style: Theme.of(context).textTheme.bodySmall
                  ?.copyWith(color: muted),
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
              onTap: () => showStatsDialog(context, stats!),
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
