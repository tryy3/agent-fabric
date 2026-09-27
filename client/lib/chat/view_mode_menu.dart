import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';
import 'chat_controller.dart';
import 'view_modes.dart';

/// Sentinel [PopupMenuButton] value that clears the thread override.
const kRestoreViewModeValue = '__app_default__';

/// Pretty / Detailed / Raw picker for the project context bar.
class ViewModeMenu extends StatelessWidget {
  const ViewModeMenu({super.key, required this.controller});

  final ChatController controller;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final threadId = controller.selectedThreadId;
    if (threadId == null) {
      return const SizedBox.shrink();
    }
    final mode = resolveViewMode(controller.selectedThread?.viewModeId);
    return PopupMenuButton<String>(
      key: const Key('view-mode-menu'),
      tooltip: 'View mode',
      enabled: !controller.sending,
      onSelected: (id) async {
        try {
          await controller.setThreadViewMode(
            id == kRestoreViewModeValue ? null : id,
          );
        } on Object catch (_) {
          if (context.mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(content: Text('Could not update view mode')),
            );
          }
        }
      },
      itemBuilder: (context) {
        final hasOverride = controller.selectedThread?.viewModeId != null;
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
                    ? Icon(Icons.check, size: 18, color: tokens.primary)
                    : const SizedBox(width: 18),
              ),
            ),
          if (hasOverride) ...[
            const PopupMenuDivider(),
            PopupMenuItem<String>(
              value: kRestoreViewModeValue,
              child: ListTile(
                contentPadding: EdgeInsets.zero,
                dense: true,
                title: const Text('Use app default'),
                subtitle: Text('Follow global default (${defaultMode.label})'),
              ),
            ),
          ],
        ];
      },
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: tokens.surface,
          border: Border.all(color: tokens.border),
          borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
        ),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: SizedBox(
            height: 36,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.layers_outlined,
                  size: 18,
                  color: tokens.textSecondary,
                ),
                const SizedBox(width: 8),
                Text(
                  mode.label,
                  style: tokens.labelMd().copyWith(color: tokens.textPrimary),
                ),
                const SizedBox(width: 6),
                Icon(Icons.expand_more, size: 16, color: tokens.textMuted),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
