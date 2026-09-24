import 'package:material_ui/material_ui.dart';

import '../dock/dock_ids.dart';
import '../dock/dock_layout_controller.dart';
import '../ui/theme/design_tokens.dart';

/// Visibility toggles for dockable core panes.
///
/// Threads live in the project sidebar, and Runs is omitted until run data exists.
class CoreViewToggles extends StatelessWidget {
  const CoreViewToggles({super.key, required this.dock, this.onToggle});

  final DockLayoutController dock;
  final ValueChanged<String>? onToggle;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: dock,
      builder: (context, _) {
        return Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(width: 12),
            _Toggle(
              dock: dock,
              coreId: DockIds.files,
              label: 'Files',
              icon: Icons.folder_outlined,
              buttonKey: const Key('toggle-files'),
              onToggle: onToggle,
            ),
            const SizedBox(width: 8),
            _Toggle(
              dock: dock,
              coreId: DockIds.chat,
              label: 'Chat',
              icon: Icons.chat_bubble_outline,
              buttonKey: const Key('toggle-chat'),
              onToggle: onToggle,
            ),
          ],
        );
      },
    );
  }
}

class _Toggle extends StatelessWidget {
  const _Toggle({
    required this.dock,
    required this.coreId,
    required this.label,
    required this.icon,
    required this.buttonKey,
    this.onToggle,
  });

  final DockLayoutController dock;
  final String coreId;
  final String label;
  final IconData icon;
  final Key buttonKey;
  final ValueChanged<String>? onToggle;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final active = dock.hasItem(coreId);
    final background = active ? tokens.primaryMuted : tokens.surface;
    final borderColor = active
        ? tokens.primary.withValues(alpha: 0.45)
        : tokens.border;
    final foreground = active ? tokens.textPrimary : tokens.textSecondary;
    final shape = RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
      side: BorderSide(color: borderColor),
    );
    final message = active ? 'Hide $label' : 'Show $label';
    return Semantics(
      button: true,
      label: message,
      toggled: active,
      child: Tooltip(
        message: message,
        child: Material(
          color: background,
          shape: shape,
          child: InkWell(
            key: buttonKey,
            customBorder: shape,
            onTap: () => (onToggle ?? dock.toggleCore)(coreId),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    icon,
                    size: 16,
                    color: active ? tokens.primary : foreground,
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
