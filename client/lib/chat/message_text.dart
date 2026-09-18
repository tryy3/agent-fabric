import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:markdown/markdown.dart' as md;
import 'package:material_ui/material_ui.dart';

import 'link_safety.dart';
import 'markdown_link_dialog.dart';

/// Chat markdown extras, based on [md.ExtensionSet.gitHubFlavored] but listed
/// explicitly so we can add/remove rules without relying on the preset name.
///
/// Core CommonMark-ish syntax (paragraphs, headings, emphasis, links, …) still
/// comes from the parser defaults; this set only adds the extras below.
final md.ExtensionSet kChatMarkdownExtensions = md.ExtensionSet(
  <md.BlockSyntax>[
    const md.FencedCodeBlockSyntax(),
    const md.TableSyntax(),
    const md.UnorderedListWithCheckboxSyntax(),
    const md.OrderedListWithCheckboxSyntax(),
    const md.FootnoteDefSyntax(),
  ],
  <md.InlineSyntax>[md.StrikethroughSyntax(), md.AutolinkExtensionSyntax()],
);

class MessageText extends StatelessWidget {
  const MessageText({
    super.key,
    required this.text,
    required this.markdown,
    this.launchLink,
  });

  final String text;
  final bool markdown;

  /// Optional override for tests; defaults to external browser via url_launcher.
  final MarkdownLinkLauncher? launchLink;

  @override
  Widget build(BuildContext context) {
    if (!markdown) {
      return Text(text);
    }
    final theme = Theme.of(context);
    final onSurface = theme.colorScheme.onSurface;
    return MarkdownBody(
      data: text,
      selectable: true,
      extensionSet: kChatMarkdownExtensions,
      onTapLink: (linkText, href, title) {
        final inspected = inspectMarkdownLink(href: href, linkText: linkText);
        showMarkdownLinkDialog(
          context,
          link: inspected,
          launchLink: launchLink,
        );
      },
      // Default package checkboxes use Material Icons tinted with
      // ThemeData.primaryColor, which matches the dark scaffold and vanishes.
      checkboxBuilder: (checked) {
        return Padding(
          padding: const EdgeInsets.only(right: 4),
          child: IgnorePointer(
            child: SizedBox(
              width: 22,
              height: 22,
              child: Checkbox(
                value: checked,
                onChanged: null,
                materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                visualDensity: VisualDensity.compact,
                side: BorderSide(color: onSurface, width: 1.5),
                fillColor: WidgetStateProperty.resolveWith((states) {
                  if (states.contains(WidgetState.selected)) {
                    return theme.colorScheme.primary;
                  }
                  return Colors.transparent;
                }),
                checkColor: theme.colorScheme.onPrimary,
              ),
            ),
          ),
        );
      },
      styleSheet: MarkdownStyleSheet(
        p: theme.textTheme.bodyMedium,
        a: theme.textTheme.bodyMedium?.copyWith(
          color: theme.colorScheme.primary,
          decoration: TextDecoration.underline,
        ),
        code: theme.textTheme.bodyMedium?.copyWith(fontFamily: 'monospace'),
        listIndent: 24,
        listBulletPadding: const EdgeInsets.only(right: 4),
        checkbox: theme.textTheme.bodyMedium?.copyWith(color: onSurface),
      ),
    );
  }
}
