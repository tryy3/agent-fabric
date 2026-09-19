import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';
import 'agent_bubble.dart';
import 'chat_bubble.dart';
import 'chat_composer.dart';
import 'chat_controller.dart';
import 'display_settings.dart';
import 'message_text.dart';
import 'view_modes.dart';

/// Sentinel [PopupMenuButton] value that clears the thread override.
const _restoreViewModeValue = '__app_default__';

class ChatScreen extends StatefulWidget {
  const ChatScreen({
    super.key,
    required this.controller,
    required this.displaySettings,
  });

  final ChatController controller;
  final ChatDisplaySettings displaySettings;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_scrollToEnd);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) {
        return;
      }
      widget.controller.connect();
    });
  }

  @override
  void dispose() {
    widget.controller.removeListener(_scrollToEnd);
    _scroll.dispose();
    super.dispose();
  }

  void _scrollToEnd() {
    if (!widget.controller.sending) {
      return;
    }
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) {
        return;
      }
      _scroll.jumpTo(_scroll.position.maxScrollExtent);
    });
  }

  Widget _contentColumn({required double width, required Widget child}) {
    return Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: width),
        child: SizedBox(width: double.infinity, child: child),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: Listenable.merge([widget.controller, widget.displaySettings]),
      builder: (context, _) {
        final c = widget.controller;
        final width = widget.displaySettings.contentWidth.toDouble();
        final mode = resolveViewMode(c.selectedThread?.viewModeId);
        final showOfflineEmpty =
            _isOffline(c.status) &&
            (c.selectedThreadId == null || c.messages.isEmpty);
        final visible = [
          for (final m in c.messages)
            if (m.kind != ChatBubbleKind.stats) m,
        ];
        return Scaffold(
          appBar: AppBar(
            title: const Text('Agent Fabric'),
            actions: [
              if (c.selectedThreadId != null)
                Padding(
                  // Keep clear of the Flutter DEBUG banner in the corner.
                  padding: const EdgeInsets.only(right: 40),
                  child: PopupMenuButton<String>(
                    key: const Key('view-mode-menu'),
                    tooltip: 'View mode',
                    enabled: !c.sending,
                    child: Material(
                      color: Theme.of(
                        context,
                      ).colorScheme.primary.withValues(alpha: 0.14),
                      borderRadius: BorderRadius.circular(20),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 8,
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.layers_outlined,
                              size: 18,
                              color: Theme.of(context).colorScheme.primary,
                            ),
                            const SizedBox(width: 6),
                            Text(
                              mode.label,
                              style: Theme.of(context).textTheme.labelLarge
                                  ?.copyWith(
                                    color: Theme.of(
                                      context,
                                    ).colorScheme.primary,
                                  ),
                            ),
                          ],
                        ),
                      ),
                    ),
                    onSelected: (id) async {
                      try {
                        await c.setThreadViewMode(
                          id == _restoreViewModeValue ? null : id,
                        );
                      } catch (_) {
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            const SnackBar(
                              content: Text('Could not update view mode'),
                            ),
                          );
                        }
                      }
                    },
                    itemBuilder: (context) {
                      final hasOverride = c.selectedThread?.viewModeId != null;
                      final defaultMode = resolveViewMode(null);
                      return [
                        for (final m in kBuiltInViewModes)
                          PopupMenuItem<String>(
                            value: m.id,
                            child: ListTile(
                              contentPadding: EdgeInsets.zero,
                              dense: true,
                              title: Text(m.label),
                              subtitle: Text(m.description),
                              trailing: m.id == mode.id
                                  ? Icon(
                                      Icons.check,
                                      size: 18,
                                      color: Theme.of(
                                        context,
                                      ).colorScheme.primary,
                                    )
                                  : const SizedBox(width: 18),
                            ),
                          ),
                        if (hasOverride) ...[
                          const PopupMenuDivider(),
                          PopupMenuItem<String>(
                            value: _restoreViewModeValue,
                            child: ListTile(
                              contentPadding: EdgeInsets.zero,
                              dense: true,
                              title: const Text('Use app default'),
                              subtitle: Text(
                                'Follow global default (${defaultMode.label})',
                              ),
                            ),
                          ),
                        ],
                      ];
                    },
                  ),
                ),
            ],
            bottom: PreferredSize(
              preferredSize: const Size.fromHeight(28),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    _statusLabel(c),
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ),
              ),
            ),
          ),
          body: Column(
            children: [
              Expanded(
                child: showOfflineEmpty
                    ? const Center(child: Text("You're offline"))
                    : c.selectedThreadId == null
                    ? const Center(
                        child: Text('Create a thread to start chatting'),
                      )
                    : ListView.builder(
                        key: const Key('message-list'),
                        controller: _scroll,
                        padding: const EdgeInsets.all(16),
                        itemCount: visible.length,
                        itemBuilder: (context, index) {
                          final m = visible[index];
                          if (m.kind == ChatBubbleKind.user) {
                            return _contentColumn(
                              width: width,
                              child: Align(
                                alignment: Alignment.centerRight,
                                child: Container(
                                  margin: const EdgeInsets.symmetric(
                                    vertical: 8,
                                  ),
                                  padding: const EdgeInsets.all(12),
                                  decoration: BoxDecoration(
                                    color: Theme.of(context)
                                        .extension<ChatColors>()!
                                        .user
                                        .fill,
                                    borderRadius: BorderRadius.circular(8),
                                  ),
                                  child: MessageText(
                                    text: m.text,
                                    markdown: mode.markdownRender,
                                  ),
                                ),
                              ),
                            );
                          }
                          ChatBubble? stats;
                          if (m.kind == ChatBubbleKind.message) {
                            final fullIndex = c.messages.indexOf(m);
                            if (fullIndex >= 0 &&
                                fullIndex + 1 < c.messages.length &&
                                c.messages[fullIndex + 1].kind ==
                                    ChatBubbleKind.stats) {
                              stats = c.messages[fullIndex + 1];
                            }
                          }
                          return _contentColumn(
                            width: width,
                            child: AgentBubble(
                              bubble: m,
                              viewMode: mode,
                              stats: stats,
                            ),
                          );
                        },
                      ),
              ),
              SafeArea(
                child: Align(
                  alignment: Alignment.center,
                  child: ConstrainedBox(
                    constraints: BoxConstraints(maxWidth: width),
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: ChatComposer(controller: c),
                    ),
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  String _statusLabel(ChatController c) {
    if (c.selectedAgentMissing) {
      return 'This agent was deleted';
    }
    if (c.selectedAgentId != null && !c.selectedAgentIsComplete) {
      return 'This agent needs a provider';
    }
    switch (c.status) {
      case ChatStatus.connecting:
        return 'Connecting…';
      case ChatStatus.reconnecting:
        return 'Reconnecting…';
      case ChatStatus.connected:
        if (c.statusMessage != null) {
          return 'Error: ${c.statusMessage}';
        }
        return 'Connected';
      case ChatStatus.error:
        return 'Error: ${c.statusMessage ?? 'unknown'}';
      case ChatStatus.disconnected:
        return 'Disconnected';
    }
  }

  bool _isOffline(ChatStatus status) {
    return status == ChatStatus.disconnected ||
        status == ChatStatus.reconnecting ||
        status == ChatStatus.error;
  }
}
