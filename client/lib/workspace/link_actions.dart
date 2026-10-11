import 'dart:async';

import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

/// Copy and Open buttons for a published URL.
class LinkCopyOpenActions extends StatelessWidget {
  const LinkCopyOpenActions({
    super.key,
    required this.url,
    required this.label,
    required this.copyKey,
    required this.openKey,
  });

  final String url;

  /// Names the link in the copy confirmation, e.g. "Site".
  final String label;
  final Key copyKey;
  final Key openKey;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        TextButton.icon(
          key: copyKey,
          onPressed: () async {
            await Clipboard.setData(ClipboardData(text: url));
            if (!context.mounted) {
              return;
            }
            ScaffoldMessenger.of(context)
                .showSnackBar(SnackBar(content: Text('Copied $label URL')));
          },
          icon: const Icon(Icons.copy, size: 16),
          label: const Text('Copy'),
        ),
        TextButton.icon(
          key: openKey,
          onPressed: () {
            final uri = Uri.tryParse(url);
            if (uri == null) {
              return;
            }
            unawaited(launchUrl(uri, mode: LaunchMode.externalApplication));
          },
          icon: const Icon(Icons.open_in_new, size: 16),
          label: const Text('Open'),
        ),
      ],
    );
  }
}
