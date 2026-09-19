import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

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
      onPressed: text.isEmpty
          ? null
          : () async {
              await Clipboard.setData(ClipboardData(text: text));
              if (!context.mounted) return;
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text(snackbarMessage)),
              );
            },
    );
  }
}
