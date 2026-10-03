import 'package:material_ui/material_ui.dart';

import '../acp/gate_info.dart';
import 'tool_status_style.dart';

/// Color of a gate risk band: quiet for safe/low, amber for elevated/high,
/// red when the gate cancelled the call.
Color gateBandColor({
  required String? band,
  required ColorScheme scheme,
  required Brightness brightness,
  required Color muted,
}) {
  return switch (band) {
    'elevated' || 'high' => toolFailedAmber(brightness),
    'cancel' => scheme.error,
    _ => muted,
  };
}

/// Compact risk pill for the tool call header, e.g. `risk 6 · elevated`.
class GateBadge extends StatelessWidget {
  const GateBadge({super.key, required this.gate});

  final GateInfo gate;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final color = gateBandColor(
      band: gate.band,
      scheme: theme.colorScheme,
      brightness: theme.brightness,
      muted: theme.colorScheme.onSurfaceVariant,
    );
    return Tooltip(
      message: 'Tool gate: ${gate.outcomeLabel} (${gate.modeLabel})',
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        decoration: BoxDecoration(
          border: Border.all(color: color.withValues(alpha: 0.6)),
          borderRadius: BorderRadius.circular(5),
        ),
        child: Text(
          gate.badgeLabel,
          style: theme.textTheme.labelSmall?.copyWith(color: color),
        ),
      ),
    );
  }
}

/// Everything the gate reported for a tool call, for the expanded view.
class GateDetails extends StatelessWidget {
  const GateDetails({super.key, required this.gate});

  final GateInfo gate;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final muted = theme.colorScheme.onSurfaceVariant;
    final labelStyle = theme.textTheme.labelLarge?.copyWith(color: muted);

    Widget row(String label, String value) {
      return Padding(
        padding: const EdgeInsets.only(bottom: 2),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(width: 92, child: Text(label, style: labelStyle)),
            Expanded(child: SelectableText(value)),
          ],
        ),
      );
    }

    final reason = gate.rationale ?? gate.reason;
    return Column(
      key: const Key('gate-details'),
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Gate', style: labelStyle),
        const SizedBox(height: 4),
        row('Mode', gate.modeLabel),
        row('Outcome', gate.outcomeLabel),
        if (gate.risk != null) row('Risk', '${gate.risk}/10 · ${gate.band}'),
        if (reason != null && reason.isNotEmpty) row('Why', reason),
        if (gate.scores.length > 1 ||
            (gate.scores.length == 1 &&
                (gate.scores.first.rationale?.isNotEmpty ?? false)))
          for (final s in gate.scores)
            row(
              s.source.isEmpty ? 'Evaluator' : s.source,
              [
                '${s.risk}/10',
                if (s.ruleId != null && s.ruleId!.isNotEmpty) s.ruleId!,
                if (s.rationale != null && s.rationale!.isNotEmpty)
                  s.rationale!,
              ].join(' · '),
            )
        else if (gate.ruleId != null && gate.ruleId!.isNotEmpty)
          row('Rule', gate.ruleId!),
      ],
    );
  }
}
