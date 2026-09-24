import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import 'link_safety.dart';

typedef MarkdownLinkLauncher = Future<bool> Function(Uri uri);

/// Shows the markdown link confirm dialog (warnings, full URL, Open / Copy).
Future<void> showMarkdownLinkDialog(
  BuildContext context, {
  required InspectedLink link,
  MarkdownLinkLauncher? launchLink,
}) async {
  if (link.displayUrl.isEmpty && link.href.isEmpty) {
    ScaffoldMessenger.of(context)
        .showSnackBar(const SnackBar(content: Text('This link has no URL.')));
    return;
  }

  await showDialog<void>(
    context: context,
    builder: (context) {
      return AlertDialog(
        title: const Text('Open link?'),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (link.warnings.isNotEmpty) ...[
                Text(
                  'Take a closer look before continuing:',
                  style: Theme.of(context).textTheme.titleSmall,
                ),
                const SizedBox(height: 8),
                for (final warning in link.warnings) ...[
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        Icons.warning_amber_rounded,
                        size: 18,
                        color: Theme.of(context).colorScheme.error,
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          warning,
                          style: Theme.of(context).textTheme.bodyMedium
                              ?.copyWith(
                                color: Theme.of(context).colorScheme.error,
                              ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 6),
                ],
                const SizedBox(height: 8),
              ],
              Text('URL', style: Theme.of(context).textTheme.labelLarge),
              const SizedBox(height: 4),
              SelectableText(
                link.displayUrl,
                style: Theme.of(context).textTheme.bodyMedium
                    ?.copyWith(fontFamily: 'monospace'),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () async {
              await Clipboard.setData(ClipboardData(text: link.displayUrl));
              if (context.mounted) {
                Navigator.of(context).pop();
                ScaffoldMessenger.of(context)
                    .showSnackBar(const SnackBar(content: Text('Link copied')));
              }
            },
            child: const Text('Copy'),
          ),
          FilledButton(
            onPressed: () async {
              final uri = link.uri;
              if (uri == null) {
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(
                      content: Text('Cannot open this link in a browser'),
                    ),
                  );
                }
                return;
              }
              final launcher =
                  launchLink ??
                  ((u) => launchUrl(u, mode: LaunchMode.externalApplication));
              final ok = await launcher(uri);
              if (context.mounted) {
                Navigator.of(context).pop();
                if (!ok) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Could not open link')),
                  );
                }
              }
            },
            child: const Text('Open'),
          ),
        ],
      );
    },
  );
}
