import 'package:material_ui/material_ui.dart';
import 'package:super_sliver_list/super_sliver_list.dart';

import '../ui/theme/chat_colors.dart';
import '../ui/theme/design_tokens.dart';
import 'agent_bubble.dart';
import 'ask_user_prompt.dart';
import 'chat_bubble.dart';
import 'chat_composer.dart';
import 'chat_controller.dart';
import 'chat_inspector.dart';
import 'copy_action.dart';
import 'display_settings.dart';
import 'message_text.dart';
import 'message_timestamp.dart';
import 'pending_interaction.dart';
import 'permission_prompt.dart';
import 'view_modes.dart';

import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';

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
      unawaited(
        widget.controller.connect().catchError((Object e, StackTrace s) {
          AppLog.record('connect: $e', s);
        }),
      );
    });
  }

  @override
  void didUpdateWidget(covariant ChatScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller.removeListener(_scrollToEnd);
      widget.controller.addListener(_scrollToEnd);
    }
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

  Widget _contentColumn({required Widget child}) {
    return SizedBox(width: double.infinity, child: child);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: Listenable.merge([widget.controller, widget.displaySettings]),
      builder: (context, _) {
        final c = widget.controller;
        final label = _statusLabel(c);
        final mode = resolveViewMode(c.selectedThread?.viewModeId);
        final showOfflineEmpty =
            _isOffline(c.status) &&
            (c.selectedThreadId == null || c.messages.isEmpty);
        final visible = [
          for (final m in c.messages)
            if (m.kind != ChatBubbleKind.stats) m,
        ];
        final tokens = designTokensOf(context);
        final pending = c.selectedPending;
        return Material(
          color: tokens.surface,
          child: Column(
            children: [
              SizedBox(
                height: 36,
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: Text(
                      label,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: tokens.caption().copyWith(
                        color: label.startsWith('Error:')
                            ? tokens.error
                            : tokens.textMuted,
                      ),
                    ),
                  ),
                ),
              ),
              Expanded(
                child: showOfflineEmpty
                    ? const Center(child: Text("You're offline"))
                    : c.selectedThreadId == null
                    ? const Center(
                        child: Text('Create a thread to start chatting'),
                      )
                    : _transcriptOrInspector(
                        controller: c,
                        mode: mode,
                        visible: visible,
                        tokens: tokens,
                      ),
              ),
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      switch (pending) {
                        final PendingPermission p => Padding(
                          padding: const EdgeInsets.only(bottom: 8),
                          child: PermissionDock(
                            pending: p,
                            onSelect: c.resolvePermission,
                          ),
                        ),
                        final PendingAskUser p => Padding(
                          padding: const EdgeInsets.only(bottom: 8),
                          child: AskUserDock(
                            key: ValueKey(p.threadId),
                            pending: p,
                            onSubmit: c.submitAskUser,
                            onSkip: c.skipAskUser,
                          ),
                        ),
                        null => const SizedBox.shrink(),
                      },
                      // Match HTML Preview: Inspector-only hides the chat input.
                      if (!(mode.rawRequests &&
                          c.surfaceMode == ChatSurfaceMode.inspector))
                        ChatComposer(controller: c),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _transcriptOrInspector({
    required ChatController controller,
    required ViewMode mode,
    required List<ChatBubble> visible,
    required DesignTokens tokens,
  }) {
    final list = _messageList(
      controller: controller,
      mode: mode,
      visible: visible,
    );
    if (!mode.rawRequests) {
      return list;
    }
    final catalog = controller.catalog;
    final threadId = controller.selectedThreadId;
    if (catalog == null || threadId == null) {
      return list;
    }
    final messageId = latestAssistantCatalogMessageId(controller.messages);
    return ChatInspectorHost(
      surfaceMode: controller.surfaceMode,
      onSurfaceMode: controller.setSurfaceMode,
      chatBody: list,
      inspector: ChatInspectorPane(
        key: ValueKey('inspector-$threadId'),
        catalog: catalog,
        threadId: threadId,
        reloadToken: messageId,
      ),
    );
  }

  Widget _messageList({
    required ChatController controller,
    required ViewMode mode,
    required List<ChatBubble> visible,
  }) {
    final c = controller;
    return SuperListView.builder(
      key: const Key('message-list'),
      controller: _scroll,
      padding: const EdgeInsets.all(16),
      itemCount: visible.length,
      itemBuilder: (context, index) {
        final m = visible[index];
        if (m.kind == ChatBubbleKind.user) {
          final muted = Theme.of(context).colorScheme.onSurfaceVariant;
          final isLastUser =
              index ==
              visible.lastIndexWhere((b) => b.kind == ChatBubbleKind.user);
          return _contentColumn(
            child: Align(
              alignment: Alignment.centerRight,
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    Container(
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
                    Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        if (m.createdAt case final created?)
                          Padding(
                            padding: const EdgeInsets.only(right: 4),
                            child: Text(
                              formatMessageTimestamp(context, created),
                              style: Theme.of(context).textTheme.bodySmall
                                  ?.copyWith(color: muted),
                            ),
                          ),
                        CopyAction(key: const Key('copy-user'), text: m.text),
                        if (isLastUser && c.canRetryLatest)
                          IconButton(
                            key: const Key('retry-user'),
                            tooltip: 'Retry',
                            icon: Icon(Icons.refresh, size: 18, color: muted),
                            visualDensity: VisualDensity.compact,
                            constraints: const BoxConstraints(
                              minWidth: 32,
                              minHeight: 32,
                            ),
                            padding: EdgeInsets.zero,
                            onPressed: () {
                              unawaited(c.retryLatest());
                            },
                          ),
                      ],
                    ),
                  ],
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
              c.messages[fullIndex + 1].kind == ChatBubbleKind.stats) {
            stats = c.messages[fullIndex + 1];
          }
        }
        return _contentColumn(
          child: AgentBubble(bubble: m, viewMode: mode, stats: stats),
        );
      },
    );
  }

  String _statusLabel(ChatController c) {
    if (c.selectedAssistantMissing) {
      return 'This assistant was deleted';
    }
    if (c.selectedAssistantId != null && !c.selectedAssistantIsComplete) {
      return 'This assistant needs a connection';
    }
    switch (c.status) {
      case ChatStatus.connecting:
        return 'Connecting...';
      case ChatStatus.reconnecting:
        return 'Reconnecting...';
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
