import 'dart:async';

import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';

/// Shows publish outcome links with copy and open actions.
Future<void> showExportResultDialog(
  BuildContext context,
  ExportPublishResult result,
) {
  return showDialog<void>(
    context: context,
    builder: (context) => ExportResultDialog(result: result),
  );
}

class ExportResultDialog extends StatelessWidget {
  const ExportResultDialog({super.key, required this.result});

  final ExportPublishResult result;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return AlertDialog(
      key: const Key('export-result-dialog'),
      title: Text(
        result.message?.isNotEmpty == true ? result.message! : 'Published',
      ),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (final link in result.links)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: _LinkRow(link: link, tokens: tokens),
              ),
          ],
        ),
      ),
      actions: [
        TextButton(
          key: const Key('export-result-done'),
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Done'),
        ),
      ],
    );
  }
}

class _LinkRow extends StatelessWidget {
  const _LinkRow({required this.link, required this.tokens});

  final ExportLink link;
  final DesignTokens tokens;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(link.label, style: tokens.labelSm()),
        const SizedBox(height: 4),
        SelectableText(
          link.url,
          style: tokens.caption().copyWith(color: tokens.textSecondary),
        ),
        const SizedBox(height: 6),
        Row(
          children: [
            TextButton.icon(
              key: Key('export-copy-${link.id}'),
              onPressed: () async {
                await Clipboard.setData(ClipboardData(text: link.url));
                if (!context.mounted) {
                  return;
                }
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text('Copied ${link.label} URL')),
                );
              },
              icon: const Icon(Icons.copy, size: 16),
              label: const Text('Copy'),
            ),
            TextButton.icon(
              key: Key('export-open-${link.id}'),
              onPressed: () {
                final uri = Uri.tryParse(link.url);
                if (uri == null) {
                  return;
                }
                unawaited(launchUrl(uri, mode: LaunchMode.externalApplication));
              },
              icon: const Icon(Icons.open_in_new, size: 16),
              label: const Text('Open'),
            ),
          ],
        ),
      ],
    );
  }
}
