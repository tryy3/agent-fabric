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
    final onSurface = theme.colorScheme.onSurface;
    return MarkdownBody(
      data: text,
      selectable: true,
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
        code: theme.textTheme.bodyMedium?.copyWith(
          fontFamily: 'monospace',
        ),
        listIndent: 24,
        listBulletPadding: const EdgeInsets.only(right: 4),
        checkbox: theme.textTheme.bodyMedium?.copyWith(color: onSurface),
      ),
    );
  }
}
