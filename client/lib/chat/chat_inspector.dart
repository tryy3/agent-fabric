import 'dart:async';

import 'package:docking/docking.dart'
    show
        Area,
        DividerPainters,
        MultiSplitView,
        MultiSplitViewTheme,
        MultiSplitViewThemeData;
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
    // Chronological oldest→newest in [captures]; rail shows newest first.
    final display = captures.reversed.toList(growable: false);
    var selectedIndex = captures.indexWhere((c) => c.id == _selectedCaptureId);
    if (selectedIndex < 0) {
      selectedIndex = captures.length - 1;
    }
    final selected = captures[selectedIndex];
    return MultiSplitViewTheme(
      data: MultiSplitViewThemeData(
        dividerThickness: 6,
        dividerPainter: DividerPainters.background(
          color: tokens.border,
          highlightedColor: tokens.borderStrong,
        ),
      ),
      child: MultiSplitView(
        axis: Axis.horizontal,
        initialAreas: [
          Area(
            size: kRequestRailDefaultWidth,
            minimalSize: kRequestRailMinWidth,
          ),
          Area(minimalSize: kInspectorContentMinWidth),
        ],
        children: [
          _RequestRail(
            scrollController: _requestScroll,
            captures: captures,
            display: display,
            selectedId: selected.id,
            onSelect: (id) => setState(() => _selectedCaptureId = id),
          ),
          _InspectorContentPane(
            tab: _tab,
            onTab: (t) => setState(() => _tab = t),
            selected: selected,
          ),
        ],
      ),
    );
  }
}

/// Default open width for the request rail (shows full labels).
const double kRequestRailDefaultWidth = 168;

/// Narrowest drag size — labels occlude under the content pane.
const double kRequestRailMinWidth = 48;

/// Intrinsic painted width of each request row (occlusion viewport).
const double kRequestRowIntrinsicWidth = 148;

/// Content pane must stay usable when the rail is wide.
const double kInspectorContentMinWidth = 200;

/// Newest-first display order for the request strip (indices into chronological list).
List<int> hopRequestDisplayOrder(int captureCount) {
  return [for (var i = captureCount - 1; i >= 0; i--) i];
}

String _twoDigits(int n) => n.toString().padLeft(2, '0');

/// `#N` for a 0-based chronological index (oldest = #1).
String hopRequestOrdinal(int chronologicalIndex) =>
    '#${chronologicalIndex + 1}';

/// `DD/MM/YY` in local time from [createdAt].
String hopRequestDateLine(DateTime createdAt) {
  final local = createdAt.toLocal();
  return '${_twoDigits(local.day)}/${_twoDigits(local.month)}/${_twoDigits(local.year % 100)}';
}

/// `HH:MM` (24h) in local time from [createdAt].
String hopRequestTimeLine(DateTime createdAt) {
  final local = createdAt.toLocal();
  return '${_twoDigits(local.hour)}:${_twoDigits(local.minute)}';
}

/// First painted line: `#N - DD/MM/YY`.
String hopRequestPrimaryLine(int chronologicalIndex, DateTime createdAt) =>
    '${hopRequestOrdinal(chronologicalIndex)} - ${hopRequestDateLine(createdAt)}';

/// Full a11y / tooltip string for one provider request hop.
String hopRequestLabel(int chronologicalIndex, DateTime createdAt) =>
    'Provider request ${hopRequestPrimaryLine(chronologicalIndex, createdAt)} ${hopRequestTimeLine(createdAt)}';

/// Paints [child] at a fixed intrinsic width; the parent viewport occludes the rest.
///
/// [OverflowBox] allows the child to exceed the rail width without Flutter's
/// constraints-overflow asserts ([UnconstrainedBox] reports those). Height must
/// be finite so this is safe inside a vertical [ListView].
class _OccludingSlot extends StatelessWidget {
  const _OccludingSlot({required this.height, required this.child});

  final double height;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: height,
      width: double.infinity,
      child: ClipRect(
        child: OverflowBox(
          alignment: Alignment.centerLeft,
          minWidth: kRequestRowIntrinsicWidth,
          maxWidth: kRequestRowIntrinsicWidth,
          minHeight: height,
          maxHeight: height,
          child: SizedBox(
            width: kRequestRowIntrinsicWidth,
            height: height,
            child: child,
          ),
        ),
      ),
    );
  }
}

/// Left request rail: fixed-intrinsic rows clipped by the split (occlusion).
class _RequestRail extends StatelessWidget {
  const _RequestRail({
    required this.scrollController,
    required this.captures,
    required this.display,
    required this.selectedId,
    required this.onSelect,
  });

  final ScrollController scrollController;
  final List<HopCapture> captures;
  final List<HopCapture> display;
  final String selectedId;
  final ValueChanged<String> onSelect;

  static const double _headerHeight = 36;
  static const double _rowHeight = 48;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return ColoredBox(
      color: tokens.sidebar,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _OccludingSlot(
            height: _headerHeight,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(10, 10, 10, 8),
              child: Text(
                'REQUESTS',
                maxLines: 1,
                softWrap: false,
                style: tokens.caption().copyWith(
                  color: tokens.textMuted,
                  letterSpacing: 0.06 * 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ),
          Divider(height: 1, color: tokens.border),
          Expanded(
            child: ListView.builder(
              key: const Key('inspector-request-rail'),
              controller: scrollController,
              clipBehavior: Clip.hardEdge,
              padding: const EdgeInsets.symmetric(vertical: 4, horizontal: 4),
              itemExtent: _rowHeight,
              itemCount: display.length,
              itemBuilder: (context, i) {
                final c = display[i];
                final chronoIndex = captures.length - 1 - i;
                return _OccludingSlot(
                  height: _rowHeight,
                  child: _RequestRailRow(
                    key: Key('inspector-request-${c.id}'),
                    chronologicalIndex: chronoIndex,
                    createdAt: c.createdAt,
                    selected: c.id == selectedId,
                    onTap: () => onSelect(c.id),
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}

class _RequestRailRow extends StatelessWidget {
  const _RequestRailRow({
    super.key,
    required this.chronologicalIndex,
    required this.createdAt,
    required this.selected,
    required this.onTap,
  });

  final int chronologicalIndex;
  final DateTime createdAt;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final primary = hopRequestPrimaryLine(chronologicalIndex, createdAt);
    final time = hopRequestTimeLine(createdAt);
    final semantics = hopRequestLabel(chronologicalIndex, createdAt);
    return Semantics(
      button: true,
      label: semantics,
      selected: selected,
      child: Tooltip(
        message: semantics,
        waitDuration: const Duration(milliseconds: 400),
        child: Material(
          color: selected ? tokens.surfaceActive : Colors.transparent,
          borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
          child: InkWell(
            onTap: onTap,
            borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Text(
                    primary,
                    maxLines: 1,
                    softWrap: false,
                    style: tokens.code().copyWith(
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                      color: selected
                          ? tokens.textPrimary
                          : tokens.textSecondary,
                      height: 1.25,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    time,
                    maxLines: 1,
                    softWrap: false,
                    style: tokens.code().copyWith(
                      fontSize: 11,
                      color: tokens.textMuted,
                      height: 1.25,
                    ),
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

/// Context | Raw underline tabs + capture body.
class _InspectorContentPane extends StatelessWidget {
  const _InspectorContentPane({
    required this.tab,
    required this.onTab,
    required this.selected,
  });

  final int tab;
  final ValueChanged<int> onTab;
  final HopCapture selected;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return ColoredBox(
      color: tokens.surface,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.only(left: 4),
            child: Row(
              children: [
                _InspectorTab(
                  label: 'Context',
                  selected: tab == 0,
                  onTap: () => onTab(0),
                ),
                _InspectorTab(
                  label: 'Raw',
                  selected: tab == 1,
                  onTap: () => onTab(1),
                ),
              ],
            ),
          ),
          Divider(height: 1, color: tokens.border),
          Expanded(
            child: tab == 0
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
      ),
    );
  }
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
      child: InkWell(
        onTap: onTap,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 44),
          child: DecoratedBox(
            decoration: BoxDecoration(
              border: Border(
                bottom: BorderSide(
                  width: 2,
                  color: selected ? tokens.primary : Colors.transparent,
                ),
              ),
            ),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              child: Text(
                label,
                style: tokens.labelSm().copyWith(
                  color: selected ? tokens.primary : tokens.textSecondary,
                  fontWeight: selected ? FontWeight.w600 : FontWeight.w500,
                ),
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
