import 'dart:async';

import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

OverlayEntry? _activeCopyToast;
Timer? _copyToastTimer;

/// Shows a compact top-right toast. Replaces any prior copy toast.
void showCopyToast(BuildContext context, String message) {
  final overlay = Overlay.maybeOf(context);
  if (overlay == null) return;

  _copyToastTimer?.cancel();
  _activeCopyToast?.remove();
  _activeCopyToast = null;

  late final OverlayEntry entry;
  entry = OverlayEntry(
    builder: (ctx) {
      final scheme = Theme.of(ctx).colorScheme;
      final top = MediaQuery.paddingOf(ctx).top + 16;
      return Positioned(
        top: top,
        right: 16,
        child: Material(
          key: const Key('copy-toast'),
          elevation: 3,
          borderRadius: BorderRadius.circular(8),
          color: scheme.inverseSurface,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            child: Text(
              message,
              style: Theme.of(ctx).textTheme.bodyMedium
                  ?.copyWith(color: scheme.onInverseSurface),
            ),
          ),
        ),
      );
    },
  );

  _activeCopyToast = entry;
  overlay.insert(entry);

  _copyToastTimer = Timer(const Duration(seconds: 2), () {
    if (_activeCopyToast != entry) return;
    entry.remove();
    _activeCopyToast = null;
    _copyToastTimer = null;
  });
}

/// Test helper: clear any active copy toast/timer.
void clearCopyToastForTest() {
  _copyToastTimer?.cancel();
  _copyToastTimer = null;
  _activeCopyToast?.remove();
  _activeCopyToast = null;
}

class CopyAction extends StatelessWidget {
  const CopyAction({
    super.key,
    required this.text,
    this.tooltip = 'Copy',
    this.snackbarMessage = 'Copied',
  });

  final String text;
  final String tooltip;
  final String snackbarMessage;

  @override
  Widget build(BuildContext context) {
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    return IconButton(
      tooltip: tooltip,
      icon: Icon(Icons.copy_outlined, size: 18, color: muted),
      visualDensity: VisualDensity.compact,
      constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
      padding: EdgeInsets.zero,
      onPressed: () async {
        if (text.isEmpty) return;
        await Clipboard.setData(ClipboardData(text: text));
        if (!context.mounted) return;
        showCopyToast(context, snackbarMessage);
      },
    );
  }
}
