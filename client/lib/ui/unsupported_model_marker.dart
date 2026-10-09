import 'package:material_ui/material_ui.dart';

import 'theme/design_tokens.dart';

const kUnsupportedModelTooltip =
    'No adapter for this model\'s wire API yet, so it cannot be selected.';

/// Dims a listed model and adds an "Unsupported" badge when [unsupported];
/// otherwise renders [child] unchanged.
class UnsupportedModelMarker extends StatelessWidget {
  const UnsupportedModelMarker({
    super.key,
    required this.unsupported,
    required this.child,
  });

  final bool unsupported;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    if (!unsupported) {
      return child;
    }
    final tokens = designTokensOf(context);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Flexible(child: Opacity(opacity: 0.5, child: child)),
        const SizedBox(width: 8),
        Tooltip(
          message: kUnsupportedModelTooltip,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
            decoration: BoxDecoration(
              color: tokens.surfaceRaised,
              border: Border.all(color: tokens.warning),
              borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
            ),
            child: Text(
              'Unsupported',
              style: TextStyle(fontSize: 11, color: tokens.warning),
            ),
          ),
        ),
      ],
    );
  }
}
