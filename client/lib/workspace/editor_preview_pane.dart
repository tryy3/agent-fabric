import 'package:docking/docking.dart' show Area, MultiSplitView;
import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';
import 'open_with.dart';
import 'web_preview_host.dart';
import 'workspace_controller.dart';

/// Text editor with a Code / Preview / Split toggle for file types that
/// have a rendered preview (HTML today). Opens in code mode; the preview
/// shows the catalog-served bytes, so it refreshes on save.
class EditorPreviewPane extends StatefulWidget {
  const EditorPreviewPane({
    super.key,
    required this.controller,
    required this.view,
    required this.editor,
  });

  final WorkspaceController controller;
  final OpenView view;
  final Widget editor;

  @override
  State<EditorPreviewPane> createState() => _EditorPreviewPaneState();
}

class _EditorPreviewPaneState extends State<EditorPreviewPane> {
  /// Latches once the preview is first shown so code ↔ preview swaps stop
  /// unmounting the iframe platform view. Mount/dispose cycles churn
  /// CanvasKit overlay surfaces ("Shader compilation error" spam on web)
  /// and reload the page; IndexedStack swaps are cheap and keep both
  /// states. Split still remounts: the preview moves into a MultiSplitView.
  bool _previewActivated = false;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final mode = widget.controller.viewModeFor(widget.view.viewId);
    if (mode != EditorViewMode.code) {
      _previewActivated = true;
    }
    final preview = _previewActivated
        ? webPreviewHostFor(widget.controller, widget.view.path)
        : const SizedBox.shrink();
    final body = switch (mode) {
      EditorViewMode.code => IndexedStack(
        index: 0,
        children: [widget.editor, preview],
      ),
      EditorViewMode.preview => IndexedStack(
        index: 1,
        children: [widget.editor, preview],
      ),
      EditorViewMode.split => MultiSplitView(
        axis: Axis.horizontal,
        initialAreas: [Area(weight: 0.5), Area(weight: 0.5)],
        children: [widget.editor, preview],
      ),
    };
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          key: const Key('editor-mode-header'),
          height: DesignTokens.tabHeight,
          padding: const EdgeInsets.symmetric(horizontal: 8),
          decoration: BoxDecoration(
            color: tokens.surface,
            border: Border(bottom: BorderSide(color: tokens.border)),
          ),
          // Reversed horizontal scroll keeps the group right-aligned and
          // clips instead of overflowing below the pane's minimum width.
          child: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            reverse: true,
            child: Row(
              children: [
                _ModeButton(
                  key: const Key('editor-mode-code'),
                  label: 'Code',
                  icon: Icons.code,
                  selected: mode == EditorViewMode.code,
                  onPressed: () => widget.controller.setViewMode(
                    widget.view.viewId,
                    EditorViewMode.code,
                  ),
                ),
                const SizedBox(width: 4),
                _ModeButton(
                  key: const Key('editor-mode-preview'),
                  label: 'Preview',
                  icon: Icons.visibility_outlined,
                  selected: mode == EditorViewMode.preview,
                  onPressed: () => widget.controller.setViewMode(
                    widget.view.viewId,
                    EditorViewMode.preview,
                  ),
                ),
                const SizedBox(width: 4),
                _ModeButton(
                  key: const Key('editor-mode-split'),
                  label: 'Split',
                  icon: Icons.vertical_split_outlined,
                  selected: mode == EditorViewMode.split,
                  onPressed: () => widget.controller.setViewMode(
                    widget.view.viewId,
                    EditorViewMode.split,
                  ),
                ),
              ],
            ),
          ),
        ),
        Expanded(child: body),
      ],
    );
  }
}

/// Toggle chip matching the core-view-toggle spec: ghost when inactive,
/// muted cyan fill when active.
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
    );
  }
}
