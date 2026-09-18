import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:material_ui/material_ui.dart';

class MessageText extends StatelessWidget {
  const MessageText({super.key, required this.text, required this.markdown});

  final String text;
  final bool markdown;

  @override
  Widget build(BuildContext context) {
    if (!markdown) {
      return Text(text);
    }
    final theme = Theme.of(context);
    return MarkdownBody(
      data: text,
      selectable: true,
      styleSheet: MarkdownStyleSheet(
        p: theme.textTheme.bodyMedium,
        code: theme.textTheme.bodyMedium?.copyWith(
          fontFamily: 'monospace',
        ),
      ),
    );
  }
}
