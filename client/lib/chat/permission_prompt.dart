import 'package:acpd/acpd.dart';
import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';
import 'pending_interaction.dart';

/// High-attention permission dock shown above the chat composer.
class PermissionDock extends StatelessWidget {
  const PermissionDock({
    super.key,
    required this.pending,
    required this.onSelect,
  });

  final PendingPermission pending;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Semantics(
      container: true,
      label: 'Permission request: ${pending.title}. ${pending.reason}',
      child: Material(
        color: tokens.surfaceRaised,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
          side: BorderSide(color: tokens.warning.withValues(alpha: 0.7)),
        ),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(12, 10, 12, 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  Icon(
                    Icons.warning_amber_rounded,
                    color: tokens.warning,
                    size: 20,
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      pending.title,
                      style: TextStyle(
                        color: tokens.textPrimary,
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 6),
              Text(
                'This tool needs your approval before it can continue.',
                style: TextStyle(color: tokens.textSecondary, fontSize: 12),
              ),
              if (pending.reason.isNotEmpty) ...[
                const SizedBox(height: 8),
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: tokens.surface,
                    borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
                    border: Border.all(color: tokens.border),
                  ),
                  child: Text(
                    pending.reason,
                    style: TextStyle(
                      color: tokens.textPrimary,
                      fontSize: 12,
                      fontFamily: 'JetBrains Mono',
                    ),
                  ),
                ),
              ],
              const SizedBox(height: 10),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                alignment: WrapAlignment.end,
                children: [
                  for (final opt in pending.request.options)
                    TextButton(
                      onPressed: () => onSelect(opt.optionId),
                      style: TextButton.styleFrom(
                        foregroundColor: switch (opt.kind) {
                          PermissionOptionKind.rejectOnce ||
                          PermissionOptionKind.rejectAlways =>
                            tokens.error,
                          PermissionOptionKind.allowAlways => tokens.primary,
                          PermissionOptionKind.allowOnce => tokens.warning,
                        },
                      ),
                      child: Text(opt.name),
                    ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
