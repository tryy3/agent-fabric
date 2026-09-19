import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';
import 'chat_bubble.dart';
import 'copy_action.dart';
import 'display_settings.dart';
import 'message_text.dart';
import 'message_timestamp.dart';
import 'stats_display.dart';
import 'tool_format.dart';
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

class _ToolCallActivityState extends State<_ToolCallActivity>
    with AutomaticKeepAliveClientMixin {
  late bool _expanded =
      widget.bubble.streamingTool ||
      widget.toolVisibility == VisibilityMode.expanded;
  int _tabIndex = 0;

  @override
  bool get wantKeepAlive => true;

  @override
  Widget build(BuildContext context) {
    super.build(context);
    final theme = Theme.of(context);
    final chat = theme.extension<ChatColors>()!;
    final muted = theme.colorScheme.onSurfaceVariant;
    final input = formatToolValue(widget.bubble.toolInput);
    final output = formatToolValue(widget.bubble.toolOutput);
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
            CopyAction(
              key: Key('copy-tool-${widget.bubble.toolCallId}'),
              text: formatToolCopyText(
                title: widget.bubble.toolTitle ?? 'Tool call',
                input: widget.bubble.toolInput,
                output: widget.bubble.toolOutput,
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
    if (!_expanded) return header;

    final outputBody = SelectableText(
      output.isEmpty ? '—' : output,
      style: const TextStyle(fontFamily: 'monospace'),
    );
    final fullBody = _FullToolBody(
      title: widget.bubble.toolTitle ?? 'Tool call',
      input: input,
      output: output,
    );
    final body = switch (widget.toolIO) {
      ToolIOMode.full => fullBody,
      ToolIOMode.output => outputBody,
      ToolIOMode.both => _tabIndex == 0 ? fullBody : outputBody,
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
                    label: 'Full',
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
            child: body,
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

class _FullToolBody extends StatelessWidget {
  const _FullToolBody({
    required this.title,
    required this.input,
    required this.output,
  });

  final String title;
  final String input;
  final String output;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final muted = theme.colorScheme.onSurfaceVariant;
    final labelStyle = theme.textTheme.labelLarge?.copyWith(color: muted);

    Widget section(String label, Widget value) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: labelStyle),
          const SizedBox(height: 4),
          value,
        ],
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        section('Tool', Text(title)),
        const SizedBox(height: 12),
        section(
          'Args',
          SelectableText(
            input.isEmpty ? '—' : input,
            style: const TextStyle(fontFamily: 'monospace'),
          ),
        ),
        const SizedBox(height: 12),
        section(
          'Output',
          SelectableText(
            output.isEmpty ? '—' : output,
            style: const TextStyle(fontFamily: 'monospace'),
          ),
        ),
      ],
    );
  }
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

class _ThoughtActivityState extends State<_ThoughtActivity>
    with AutomaticKeepAliveClientMixin {
  late bool _expanded =
      widget.bubble.streamingThought ||
      widget.thinkingVisibility == VisibilityMode.expanded;

  @override
  bool get wantKeepAlive => true;

  @override
  Widget build(BuildContext context) {
    super.build(context);
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
            CopyAction(
              key: const Key('copy-thinking'),
              text: widget.bubble.text,
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
        if (caption.isNotEmpty || bubble.text.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Row(
              children: [
                if (caption.isNotEmpty)
                  Expanded(
                    child: Text(
                      caption,
                      style: Theme.of(context).textTheme.bodySmall
                          ?.copyWith(color: muted),
                    ),
                  )
                else
                  const Spacer(),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (bubble.createdAt case final created?)
                      Padding(
                        padding: const EdgeInsets.only(right: 4),
                        child: Text(
                          formatMessageTimestamp(context, created),
                          style: Theme.of(
                            context,
                          ).textTheme.bodySmall?.copyWith(color: muted),
                        ),
                      ),
                    CopyAction(
                      key: const Key('copy-message'),
                      text: bubble.text,
                    ),
                  ],
                ),
              ],
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
