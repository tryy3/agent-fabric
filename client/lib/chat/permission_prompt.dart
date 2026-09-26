import 'package:acpd/acpd.dart';
import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';

/// High-attention permission prompt for ACP [session/request_permission].
Future<RequestPermissionResponse> showPermissionPrompt(
  BuildContext context,
  RequestPermissionRequest request,
) async {
  final tokens = designTokensOf(context);
  final title = request.toolCall.title ?? 'Permission required';
  final reason = _permissionReason(request.toolCall.rawInput);

  final optionId = await showDialog<String>(
    context: context,
    barrierDismissible: false,
    builder: (ctx) {
      return AlertDialog(
        backgroundColor: tokens.surfaceRaised,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
          side: BorderSide(color: tokens.warning.withValues(alpha: 0.7)),
        ),
        title: Row(
          children: [
            Icon(Icons.warning_amber_rounded, color: tokens.warning, size: 22),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                title,
                style: TextStyle(
                  color: tokens.textPrimary,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ],
        ),
        content: Semantics(
          container: true,
          label: 'Permission request: $title. $reason',
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'This tool needs your approval before it can continue.',
                  style: TextStyle(color: tokens.textSecondary, fontSize: 13),
                ),
                if (reason.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  Container(
                    width: double.infinity,
                    padding: const EdgeInsets.all(10),
                    decoration: BoxDecoration(
                      color: tokens.surface,
                      borderRadius: BorderRadius.circular(
                        DesignTokens.radiusSm,
                      ),
                      border: Border.all(color: tokens.border),
                    ),
                    child: Text(
                      reason,
                      style: TextStyle(
                        color: tokens.textPrimary,
                        fontSize: 13,
                        fontFamily: 'JetBrains Mono',
                      ),
                    ),
                  ),
                ],
              ],
            ),
          ),
        ),
        actions: [
          for (final opt in request.options)
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(opt.optionId),
              style: TextButton.styleFrom(
                foregroundColor: switch (opt.kind) {
                  PermissionOptionKind.rejectOnce ||
                  PermissionOptionKind.rejectAlways => tokens.error,
                  PermissionOptionKind.allowAlways => tokens.primary,
                  PermissionOptionKind.allowOnce => tokens.warning,
                },
              ),
              child: Text(opt.name),
            ),
        ],
      );
    },
  );

  if (optionId == null) {
    return const RequestPermissionResponse(outcome: PermissionCancelled());
  }
  return RequestPermissionResponse(
    outcome: PermissionSelected(optionId: optionId),
  );
}

String _permissionReason(Object? rawInput) {
  if (rawInput is Map) {
    final reason = rawInput['reason'];
    if (reason is String && reason.trim().isNotEmpty) {
      return reason.trim();
    }
    final path = rawInput['path'];
    if (path is String && path.trim().isNotEmpty) {
      return 'Path: ${path.trim()}';
    }
  }
  return '';
}
