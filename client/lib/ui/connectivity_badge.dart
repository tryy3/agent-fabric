import 'package:material_ui/material_ui.dart';

import '../chat/chat_controller.dart';

/// Compact ACP connectivity indicator with an optional Retry action.
class ConnectivityBadge extends StatelessWidget {
  const ConnectivityBadge({super.key, required this.status, this.onRetry});

  final ChatStatus status;

  /// Invoked when the user taps Retry while offline or reconnecting.
  final VoidCallback? onRetry;

  bool get _canRetry =>
      onRetry != null &&
      (status == ChatStatus.reconnecting ||
          status == ChatStatus.disconnected ||
          status == ChatStatus.error);

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final (label, color, tooltip) = switch (status) {
      ChatStatus.connected => ('Online', scheme.tertiary, 'Connected'),
      ChatStatus.connecting => (
        'Reconnecting...',
        scheme.secondary,
        'Connecting…',
      ),
      ChatStatus.reconnecting => (
        'Reconnecting...',
        scheme.secondary,
        'Waiting to retry…',
      ),
      ChatStatus.disconnected ||
      ChatStatus.error => ('Offline', scheme.error, 'Disconnected'),
    };

    return Row(
      children: [
        Expanded(
          child: Tooltip(
            message: tooltip,
            child: Semantics(
              label: 'Connectivity: $label',
              child: Row(
                children: [
                  Icon(Icons.circle, size: 10, color: color),
                  const SizedBox(width: 6),
                  Flexible(
                    child: Text(
                      label,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.labelMedium,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
        if (_canRetry) ...[
          const SizedBox(width: 4),
          _RetryButton(onRetry: onRetry!),
        ],
      ],
    );
  }
}

class _RetryButton extends StatelessWidget {
  const _RetryButton({required this.onRetry});

  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Semantics(
      button: true,
      label: 'Retry connection',
      child: Tooltip(
        message: 'Retry now',
        child: Material(
          type: MaterialType.transparency,
          child: InkWell(
            onTap: onRetry,
            borderRadius: BorderRadius.circular(6),
            child: ConstrainedBox(
              constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 6),
                child: Center(
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      ExcludeSemantics(
                        child: Icon(
                          Icons.refresh,
                          size: 16,
                          color: scheme.secondary,
                        ),
                      ),
                      const SizedBox(width: 4),
                      Text(
                        'Retry',
                        style: Theme.of(context).textTheme.labelMedium
                            ?.copyWith(color: scheme.secondary),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
