import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:material_ui/material_ui.dart';

import '../acp/permission_modes.dart';
import '../ui/theme/design_tokens.dart';
import 'chat_controller.dart';

/// Composer control for the session's permission mode. The choice applies to
/// this chat session only: a new session starts in the assistant's mode, and
/// Full access asks for confirmation every time it is chosen.
class PermissionModePicker extends StatelessWidget {
  const PermissionModePicker({super.key, required this.controller});

  final ChatController controller;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final modes = controller.permissionModes;
    final currentId = controller.currentPermissionMode;
    PermissionModeOption? current;
    for (final m in modes) {
      if (m.id == currentId) current = m;
    }
    final full = current?.isFull ?? false;
    final color = full ? tokens.error : tokens.textSecondary;
    final enabled = controller.canSelectPermissionMode;
    return PopupMenuButton<String>(
      key: const Key('permission-mode-picker'),
      enabled: enabled,
      tooltip: 'Tool permissions for this session',
      initialValue: currentId,
      position: PopupMenuPosition.over,
      onSelected: (id) => unawaited(
        _select(context, id).catchError((Object e, StackTrace s) {
          AppLog.record('permission mode select: $e', s);
        }),
      ),
      itemBuilder: (context) => [
        for (final m in modes)
          PopupMenuItem<String>(
            key: Key('permission-mode-${m.id}'),
            value: m.id,
            child: _ModeItem(mode: m, selected: m.id == currentId),
          ),
      ],
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 6),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              full ? Icons.gpp_maybe_outlined : Icons.shield_outlined,
              size: 16,
              color: enabled ? color : tokens.textMuted,
            ),
            const SizedBox(width: 4),
            Flexible(
              child: Text(
                current?.name ?? 'Permissions',
                key: const Key('permission-mode-label'),
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: enabled ? color : tokens.textMuted,
                ),
              ),
            ),
            Icon(
              Icons.arrow_drop_down,
              size: 18,
              color: enabled ? tokens.textSecondary : tokens.textMuted,
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _select(BuildContext context, String id) async {
    if (id == controller.currentPermissionMode) return;
    var confirmed = false;
    if (id == kPermissionModeFull) {
      confirmed =
          await showDialog<bool>(
            context: context,
            builder: (context) => const FullAccessConfirmDialog(),
          ) ??
          false;
      if (!confirmed) return;
    }
    await controller.selectPermissionMode(id, confirmFullAccess: confirmed);
  }
}

class _ModeItem extends StatelessWidget {
  const _ModeItem({required this.mode, required this.selected});

  final PermissionModeOption mode;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 320),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 24,
            child: selected
                ? Icon(Icons.check, size: 16, color: tokens.primary)
                : null,
          ),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  mode.name,
                  style: TextStyle(
                    fontWeight: FontWeight.w500,
                    color: mode.isFull ? tokens.error : tokens.textPrimary,
                  ),
                ),
                if (mode.description case final description?)
                  Text(
                    description,
                    style: TextStyle(fontSize: 12, color: tokens.textMuted),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Asks the user to confirm Full access before the session switches to it.
class FullAccessConfirmDialog extends StatelessWidget {
  const FullAccessConfirmDialog({super.key});

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return AlertDialog(
      key: const Key('full-access-confirm'),
      icon: Icon(Icons.gpp_maybe_outlined, color: tokens.error),
      title: const Text('Turn on Full access?'),
      content: const SizedBox(
        width: 420,
        child: Text(
          'The agent will run every tool call without asking, including '
          'deletes, commands that change your project and reads or writes '
          'outside the sandbox policy. Only calls that reach Full '
          'access\'s cancel threshold are still cancelled.\n\n'
          'Full access lasts for this chat session only. A new session, a '
          'reload or a reconnect starts in the assistant\'s own mode again.',
        ),
      ),
      actions: [
        TextButton(
          key: const Key('full-access-cancel'),
          onPressed: () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const Key('full-access-confirm-button'),
          style: FilledButton.styleFrom(
            backgroundColor: tokens.error,
            foregroundColor: tokens.background,
          ),
          onPressed: () => Navigator.of(context).pop(true),
          child: const Text('Turn on Full access'),
        ),
      ],
    );
  }
}
