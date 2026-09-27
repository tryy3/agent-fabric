import 'dart:async';

import 'package:docking/docking.dart' show Area, MultiSplitView;
import 'package:flutter/gestures.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';
import '../workspace/editors/read_only_code_view.dart';
import 'chat_bubble.dart';
import 'inspector_http_view.dart';

/// How the chat core combines transcript and hop Inspector (Raw mode only).
enum ChatSurfaceMode { chat, inspector, split }

/// Chat | Inspector | Split toggle for the Chat tab toolbar (Raw mode).
class ChatSurfaceModeToggle extends StatelessWidget {
  const ChatSurfaceModeToggle({
    super.key,
    required this.mode,
    required this.onMode,
  });

  final ChatSurfaceMode mode;
  final ValueChanged<ChatSurfaceMode> onMode;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        _ModeButton(
          key: const Key('chat-surface-chat'),
          label: 'Chat',
          icon: Icons.chat_bubble_outline,
          selected: mode == ChatSurfaceMode.chat,
          onPressed: () => onMode(ChatSurfaceMode.chat),
        ),
        const SizedBox(width: 4),
        _ModeButton(
          key: const Key('chat-surface-inspector'),
          label: 'Inspector',
          icon: Icons.manage_search_outlined,
          selected: mode == ChatSurfaceMode.inspector,
          onPressed: () => onMode(ChatSurfaceMode.inspector),
        ),
        const SizedBox(width: 4),
        _ModeButton(
          key: const Key('chat-surface-split'),
          label: 'Split',
          icon: Icons.vertical_split_outlined,
          selected: mode == ChatSurfaceMode.split,
          onPressed: () => onMode(ChatSurfaceMode.split),
        ),
      ],
    );
  }
}

/// Hop Inspector panel: thread hop list + Context / Raw tabs.
///
/// [reloadToken] should change when a new assistant turn is committed so the
/// pane refetches; typically the latest assistant catalog message id.
class ChatInspectorPane extends StatefulWidget {
  const ChatInspectorPane({
    super.key,
    required this.catalog,
    required this.threadId,
    this.reloadToken,
  });

  final CatalogClient catalog;
  final String threadId;
  final String? reloadToken;

  @override
  State<ChatInspectorPane> createState() => _ChatInspectorPaneState();
}

class _ChatInspectorPaneState extends State<ChatInspectorPane> {
  List<HopCapture>? _captures;
  Object? _error;
  bool _loading = false;
  String? _selectedCaptureId;
  int _tab = 0; // 0 context, 1 raw
  final ScrollController _requestScroll = ScrollController();

  @override
  void initState() {
    super.initState();
    unawaited(_load(selectNewest: true));
  }

  @override
  void dispose() {
    _requestScroll.dispose();
    super.dispose();
  }

  @override
  void didUpdateWidget(covariant ChatInspectorPane oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.threadId != widget.threadId) {
      unawaited(_load(selectNewest: true));
      return;
    }
    if (oldWidget.reloadToken != widget.reloadToken) {
      unawaited(_load(selectNewest: true));
    }
  }

  Future<void> _load({required bool selectNewest}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await widget.catalog.listThreadCaptures(widget.threadId);
      if (!mounted) {
        return;
      }
      final keepId = _selectedCaptureId;
      final stillThere = keepId != null && list.any((c) => c.id == keepId);
      setState(() {
        _captures = list;
        if (list.isEmpty) {
          _selectedCaptureId = null;
        } else if (selectNewest || !stillThere) {
          _selectedCaptureId = list.last.id;
        }
        _loading = false;
      });
      if (selectNewest) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (_requestScroll.hasClients) {
            _requestScroll.jumpTo(0);
          }
        });
      }
    } on Object catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  void _onRequestStripPointerSignal(PointerSignalEvent event) {
    if (event is! PointerScrollEvent || !_requestScroll.hasClients) {
      return;
    }
    // Mouse wheels report vertical deltas; remap onto the horizontal strip.
    // Leave dx-only (trackpad) to the ListView so we don't double-apply.
    if (event.scrollDelta.dy == 0) {
      return;
    }
    GestureBinding.instance.pointerSignalResolver.register(event, (resolved) {
      final scroll = resolved as PointerScrollEvent;
      if (!_requestScroll.hasClients) {
        return;
      }
      final next = (_requestScroll.offset + scroll.scrollDelta.dy).clamp(
        0.0,
        _requestScroll.position.maxScrollExtent,
      );
      _requestScroll.jumpTo(next);
    });
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    if (_loading && _captures == null) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null && (_captures == null || _captures!.isEmpty)) {
      return Center(
        child: Text(
          'Could not load captures',
          style: tokens.bodySm().copyWith(color: tokens.error),
        ),
      );
    }
    final captures = _captures ?? const <HopCapture>[];
    if (captures.isEmpty) {
      return Center(
        child: Text(
          'No captures yet — send a prompt in Raw mode',
          style: tokens.bodySm().copyWith(color: tokens.textMuted),
          textAlign: TextAlign.center,
        ),
      );
    }
    // Chronological oldest→newest in [captures]; strip shows newest first.
    final display = captures.reversed.toList(growable: false);
    var selectedIndex = captures.indexWhere((c) => c.id == _selectedCaptureId);
    if (selectedIndex < 0) {
      selectedIndex = captures.length - 1;
    }
    final selected = captures[selectedIndex];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: 40,
          child: Listener(
            onPointerSignal: _onRequestStripPointerSignal,
            child: ListView.separated(
              controller: _requestScroll,
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
              itemCount: display.length,
              separatorBuilder: (_, _) => const SizedBox(width: 6),
              itemBuilder: (context, i) {
                final c = display[i];
                final chronoIndex = captures.length - 1 - i;
                final selectedHop = c.id == selected.id;
                final label = hopRequestLabel(chronoIndex);
                return Semantics(
                  button: true,
                  label: label,
                  selected: selectedHop,
                  child: Material(
                    color: selectedHop
                        ? tokens.surfaceActive
                        : tokens.surfaceRaised,
                    borderRadius: BorderRadius.circular(5),
                    child: InkWell(
                      onTap: () => setState(() => _selectedCaptureId = c.id),
                      borderRadius: BorderRadius.circular(5),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 10,
                          vertical: 6,
                        ),
                        child: Text(
                          label,
                          style: tokens.labelSm().copyWith(
                            color: tokens.textSecondary,
                          ),
                        ),
                      ),
                    ),
                  ),
                );
              },
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8),
          child: Row(
            children: [
              _InspectorTab(
                label: 'Context',
                selected: _tab == 0,
                onTap: () => setState(() => _tab = 0),
              ),
              const SizedBox(width: 4),
              _InspectorTab(
                label: 'Raw',
                selected: _tab == 1,
                onTap: () => setState(() => _tab = 1),
              ),
            ],
          ),
        ),
        const Divider(height: 1),
        Expanded(
          child: _tab == 0
              ? KeyedSubtree(
                  key: ValueKey('context-${selected.id}'),
                  child: ReadOnlyCodeView(
                    text: inspectorContextText(selected),
                    languageId: 'json',
                  ),
                )
              : KeyedSubtree(
                  key: ValueKey('raw-${selected.id}'),
                  child: InspectorHttpView(capture: selected),
                ),
        ),
      ],
    );
  }
}

/// 1-based request label for a capture in chronological order (oldest = R1).
String hopRequestLabel(int chronologicalIndex) => 'R${chronologicalIndex + 1}';

/// Newest-first display order for the request strip (indices into chronological list).
List<int> hopRequestDisplayOrder(int captureCount) {
  return [for (var i = captureCount - 1; i >= 0; i--) i];
}

class _InspectorTab extends StatelessWidget {
  const _InspectorTab({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Semantics(
      button: true,
      label: label,
      selected: selected,
      child: Material(
        color: selected ? tokens.primaryMuted : Colors.transparent,
        borderRadius: BorderRadius.circular(5),
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(5),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: Text(
              label,
              style: tokens.labelSm().copyWith(
                color: selected ? tokens.primary : tokens.textSecondary,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Chat | Inspector | Split chrome in the Chat tab (Raw mode).
///
/// Opens in Chat; Inspector stays mounted after first show so Chat ↔ Inspector
/// swaps keep scroll/selection. Split remounts into a MultiSplitView.
class ChatInspectorHost extends StatefulWidget {
  const ChatInspectorHost({
    super.key,
    required this.surfaceMode,
    required this.onSurfaceMode,
    required this.chatBody,
    required this.inspector,
  });

  final ChatSurfaceMode surfaceMode;
  final ValueChanged<ChatSurfaceMode> onSurfaceMode;
  final Widget chatBody;
  final Widget inspector;

  @override
  State<ChatInspectorHost> createState() => _ChatInspectorHostState();
}

class _ChatInspectorHostState extends State<ChatInspectorHost> {
  bool _inspectorActivated = false;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final mode = widget.surfaceMode;
    if (mode != ChatSurfaceMode.chat) {
      _inspectorActivated = true;
    }
    final inspector = _inspectorActivated
        ? widget.inspector
        : const SizedBox.shrink();
    final body = switch (mode) {
      ChatSurfaceMode.chat => IndexedStack(
        index: 0,
        children: [widget.chatBody, inspector],
      ),
      ChatSurfaceMode.inspector => IndexedStack(
        index: 1,
        children: [widget.chatBody, inspector],
      ),
      ChatSurfaceMode.split => MultiSplitView(
        axis: Axis.horizontal,
        initialAreas: [Area(weight: 0.5), Area(weight: 0.5)],
        children: [widget.chatBody, inspector],
      ),
    };
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          key: const Key('chat-surface-header'),
          height: DesignTokens.tabHeight,
          padding: const EdgeInsets.symmetric(horizontal: 8),
          decoration: BoxDecoration(
            color: tokens.surface,
            border: Border(bottom: BorderSide(color: tokens.border)),
          ),
          child: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            reverse: true,
            child: ChatSurfaceModeToggle(
              mode: mode,
              onMode: widget.onSurfaceMode,
            ),
          ),
        ),
        Expanded(child: body),
      ],
    );
  }
}

/// Toggle chip matching EditorPreviewPane: ghost when inactive, muted cyan when active.
class _ModeButton extends StatelessWidget {
  const _ModeButton({
    super.key,
    required this.label,
    required this.icon,
    required this.selected,
    required this.onPressed,
  });

  final String label;
  final IconData icon;
  final bool selected;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final background = selected ? tokens.primaryMuted : tokens.surface;
    final borderColor = selected
        ? tokens.primary.withValues(alpha: 0.45)
        : tokens.border;
    final foreground = selected ? tokens.textPrimary : tokens.textSecondary;
    final shape = RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
      side: BorderSide(color: borderColor),
    );
    return Tooltip(
      message: label,
      child: Semantics(
        button: true,
        label: label,
        selected: selected,
        child: Material(
          color: background,
          shape: shape,
          child: InkWell(
            customBorder: shape,
            onTap: onPressed,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    icon,
                    size: 16,
                    color: selected ? tokens.primary : foreground,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    label,
                    style: tokens.labelMd().copyWith(color: foreground),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

String? latestAssistantCatalogMessageId(List<ChatBubble> messages) {
  for (var i = messages.length - 1; i >= 0; i--) {
    final m = messages[i];
    if (m.kind == ChatBubbleKind.message &&
        m.catalogMessageId != null &&
        m.catalogMessageId!.isNotEmpty) {
      return m.catalogMessageId;
    }
  }
  return null;
}
