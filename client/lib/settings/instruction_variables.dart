import 'package:agent_fabric_client/ui/theme/design_tokens.dart';
import 'package:material_ui/material_ui.dart';

/// One instruction variable the plane substitutes at session/new.
class InstructionVariable {
  const InstructionVariable({
    required this.token,
    required this.meaning,
    required this.example,
    required this.notes,
  });

  /// Placeholder syntax written into instruction sources, e.g. `{{modelId}}`.
  final String token;

  /// Operator-facing explanation of what the plane substitutes.
  final String meaning;

  /// Representative substituted value for teaching.
  final String example;

  /// Pin-time / edge-case detail shown under the meaning.
  final String notes;
}

/// Catalog of known instruction variables (matches plane `ApplyInstructionVars`).
const List<InstructionVariable> kInstructionVariables = [
  InstructionVariable(
    token: '{{currentDate}}',
    meaning: 'Civil date when the session is pinned (host local calendar day).',
    example: '2026-10-02',
    notes: 'YYYY-MM-DD. Static for the life of the session.',
  ),
  InstructionVariable(
    token: '{{timezone}}',
    meaning: 'IANA / zone name for the host clock used at pin time.',
    example: 'Europe/Stockholm',
    notes: 'Falls back to a zone abbreviation or UTC if Location is Local.',
  ),
  InstructionVariable(
    token: '{{workspaceRoot}}',
    meaning: 'Resolved workspace root for the bound thread’s project.',
    example: '/workspace',
    notes: 'Defaults to /workspace if no project root is resolved.',
  ),
  InstructionVariable(
    token: '{{modelId}}',
    meaning: 'Model id selected for this session at pin time.',
    example: 'gpt-5',
    notes:
        'Tracks the session’s current model, not a later picker change '
        'mid-thread.',
  ),
];

/// Sticky reference rail listing instruction variables with meaning + example.
///
/// Selecting a row updates the detail panel. [onInsert] inserts the selected
/// token into the focused instruction field when provided.
class InstructionVariablesRail extends StatelessWidget {
  const InstructionVariablesRail({
    super.key,
    required this.selectedToken,
    required this.onSelect,
    this.onInsert,
  });

  final String selectedToken;
  final ValueChanged<String> onSelect;
  final VoidCallback? onInsert;

  InstructionVariable get _selected {
    for (final variable in kInstructionVariables) {
      if (variable.token == selectedToken) {
        return variable;
      }
    }
    return kInstructionVariables.first;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final tokens = designTokensOf(context);
    final selected = _selected;

    return DecoratedBox(
      decoration: BoxDecoration(
        color: tokens.surface,
        border: Border.all(color: tokens.border),
        borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Variables', style: theme.textTheme.titleSmall),
            const SizedBox(height: 6),
            Text(
              'Substituted when effective instructions are pinned at '
              'session/new. Available in every instruction source.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: tokens.textMuted,
              ),
            ),
            const SizedBox(height: 12),
            for (final variable in kInstructionVariables) ...[
              _VariableRow(
                variable: variable,
                selected: variable.token == selected.token,
                onTap: () => onSelect(variable.token),
              ),
              const SizedBox(height: 4),
            ],
            const SizedBox(height: 8),
            Divider(height: 1, color: tokens.border),
            const SizedBox(height: 12),
            Text(
              'Meaning',
              style: theme.textTheme.labelMedium?.copyWith(
                color: tokens.textSecondary,
              ),
            ),
            const SizedBox(height: 4),
            Text(selected.meaning, style: theme.textTheme.bodySmall),
            const SizedBox(height: 12),
            Text(
              'Example',
              style: theme.textTheme.labelMedium?.copyWith(
                color: tokens.textSecondary,
              ),
            ),
            const SizedBox(height: 4),
            SelectableText(
              selected.example,
              style: theme.textTheme.bodySmall?.copyWith(
                fontFamily: 'JetBrains Mono',
                color: tokens.textPrimary,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              selected.notes,
              style: theme.textTheme.bodySmall?.copyWith(
                color: tokens.textMuted,
              ),
            ),
            if (onInsert != null) ...[
              const SizedBox(height: 12),
              Align(
                alignment: Alignment.centerLeft,
                child: Semantics(
                  button: true,
                  label: 'Insert ${selected.token}',
                  child: OutlinedButton(
                    key: const Key('instruction-variable-insert'),
                    onPressed: onInsert,
                    child: Text('Insert ${selected.token}'),
                  ),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _VariableRow extends StatelessWidget {
  const _VariableRow({
    required this.variable,
    required this.selected,
    required this.onTap,
  });

  final InstructionVariable variable;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final tokens = designTokensOf(context);

    return Semantics(
      button: true,
      selected: selected,
      label: variable.token,
      child: Material(
        color: selected ? tokens.surfaceActive : Colors.transparent,
        borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
        child: InkWell(
          key: Key('instruction-variable-${variable.token}'),
          onTap: onTap,
          borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 44),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
              child: Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  variable.token,
                  style: theme.textTheme.bodySmall?.copyWith(
                    fontFamily: 'JetBrains Mono',
                    color: selected ? tokens.textPrimary : tokens.textSecondary,
                    fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
