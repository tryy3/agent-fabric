import 'package:material_ui/material_ui.dart';

/// A titled list of label/value rows with a footnote, used for thread and
/// round cost details.
class CostDetailsDialog extends StatelessWidget {
  const CostDetailsDialog({
    super.key,
    required this.title,
    required this.rows,
    required this.footnote,
  });

  final String title;
  final List<(String, String)> rows;
  final String footnote;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(title),
      content: SizedBox(
        width: 320,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (final (label, value) in rows)
                ListTile(
                  dense: true,
                  visualDensity: VisualDensity.compact,
                  contentPadding: EdgeInsets.zero,
                  title: Text(
                    label,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  subtitle: SelectableText(value),
                ),
              const SizedBox(height: 8),
              Text(footnote, style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Close'),
        ),
      ],
    );
  }
}
