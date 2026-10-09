import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';

/// Tools a permission rule can target; `*` is every tool with a path or command.
const permissionRuleTools = <String>[
  'run_command',
  'read_file',
  'write_file',
  'append_file',
  'apply_patch',
  'create_directory',
  'move_path',
  'delete_path',
  'list_files',
  'search_text',
  '*',
];

/// The score a new rule starts with: the elevated band, so the permission
/// mode asks in every mode but the most permissive.
const permissionRuleDefaultScore = 5;

/// The band name of a risk score (decision 22).
String permissionBandLabel(int risk) => switch (risk) {
  <= 2 => 'safe',
  <= 4 => 'low',
  <= 6 => 'elevated',
  <= 8 => 'high',
  _ => 'cancel',
};

/// The score picker of a permission rule and of a built-in rule: 1-10, each
/// with its band. [defaultScore] marks a built-in rule's own score.
class PermissionScoreDropdown extends StatelessWidget {
  const PermissionScoreDropdown({
    super.key,
    required this.fieldKey,
    required this.value,
    required this.onChanged,
    this.defaultScore,
  });

  final Key fieldKey;
  final int value;
  final ValueChanged<int> onChanged;
  final int? defaultScore;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return DropdownButtonFormField<int>(
      key: fieldKey,
      initialValue: value,
      isExpanded: true,
      decoration: const InputDecoration(
        labelText: 'Score',
        border: OutlineInputBorder(),
        isDense: true,
      ),
      items: [
        for (var r = 1; r <= 10; r++)
          DropdownMenuItem(
            value: r,
            child: Text(
              '$r · ${permissionBandLabel(r)}'
              '${r == defaultScore ? ' (default)' : ''}',
              style: TextStyle(
                color: r >= 9 ? tokens.error : tokens.textPrimary,
              ),
            ),
          ),
      ],
      onChanged: (v) {
        if (v != null) onChanged(v);
      },
    );
  }
}

/// What "Ask the scorers" means, shown beside every switch.
const askScorersHelp =
    'When on, the fast and deep scorers set up for the assistant also look at '
    'calls this rule matches. They can raise the score, and the deep scorer '
    'can lower it a little, so the final score may differ from the one set '
    'here. When off, the score is final. Without scorers set up this does '
    'nothing.';

/// The "Ask the scorers" switch of a permission rule or a built-in rule, with
/// its explanation.
class AskScorersSwitch extends StatelessWidget {
  const AskScorersSwitch({
    super.key,
    required this.switchKey,
    required this.value,
    required this.onChanged,
  });

  final Key switchKey;
  final bool value;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Switch(key: switchKey, value: value, onChanged: onChanged),
        const SizedBox(width: 4),
        const Text('Ask the scorers'),
        const SizedBox(width: 4),
        Tooltip(
          message: askScorersHelp,
          triggerMode: TooltipTriggerMode.tap,
          showDuration: const Duration(seconds: 8),
          child: Icon(Icons.info_outline, size: 16, color: tokens.textMuted),
        ),
      ],
    );
  }
}

/// System One answer strategies for the fast scorer.
const permissionScorerStrategies = <String>['score', 'bands'];

/// One editable permission rule row: a tool, a pattern and the score a
/// matching call gets, exactly like a built-in rule.
class PermissionRuleDraft {
  PermissionRuleDraft({
    this.tool = 'run_command',
    String match = '',
    this.risk = permissionRuleDefaultScore,
    this.consult = false,
  }) : match = TextEditingController(text: match);

  String tool;
  final TextEditingController match;
  int risk;

  /// Whether the gate scorers may look at a matching call, which can raise
  /// (or, for the deep scorer, slightly lower) the rule's score.
  bool consult;

  /// Reads a stored rule. Rules saved with an allow, ask or deny action show
  /// as the score that action implies (1, the built-in score, 10) and are
  /// stored as score rules on the next save.
  factory PermissionRuleDraft.fromJson(Map<String, Object?> json) {
    final stored = (json['risk'] as num?)?.toInt();
    return PermissionRuleDraft(
      tool: json['tool'] as String? ?? 'run_command',
      match: json['match'] as String? ?? '',
      risk:
          stored ??
          switch (json['action']) {
            'allow' => 1,
            'deny' => 10,
            _ => permissionRuleDefaultScore,
          },
      consult: json['consult'] == true,
    );
  }

  Map<String, Object?> toJson() => {
    'tool': tool,
    'match': match.text.trim(),
    'action': 'score',
    'risk': risk,
    if (consult) 'consult': true,
  };

  void dispose() => match.dispose();
}

/// One editable scorer tier (fast or deep) of the gate cascade.
class PermissionScorerDraft {
  PermissionScorerDraft({this.connectionId, String model = '', this.strategy})
    : model = TextEditingController(text: model);

  String? connectionId;
  final TextEditingController model;
  String? strategy;

  factory PermissionScorerDraft.fromJson(Object? raw) {
    if (raw is! Map) return PermissionScorerDraft();
    return PermissionScorerDraft(
      connectionId: raw['connectionId'] as String?,
      model: raw['model'] as String? ?? '',
      strategy: raw['strategy'] as String?,
    );
  }

  bool get isSet => connectionId != null && model.text.trim().isNotEmpty;

  /// Null removes the tier from the stored settings.
  Map<String, Object?>? toJson() {
    if (!isSet) return null;
    return {
      'connectionId': connectionId,
      'model': model.text.trim(),
      if (strategy != null) 'strategy': strategy,
    };
  }

  void dispose() => model.dispose();
}

/// The cascade options stored beside the scorer tiers. Only `maxLower` is
/// edited here; `minConfidence` and `skipAtOrBelow` are kept as stored.
class PermissionScorerTuning {
  PermissionScorerTuning({
    int? maxLower,
    this.minConfidence,
    this.skipAtOrBelow,
  }) : maxLower = TextEditingController(text: maxLower?.toString() ?? '');

  /// How many points the deep scorer may lower a score; empty is the default.
  final TextEditingController maxLower;
  final Object? minConfidence;
  final Object? skipAtOrBelow;

  factory PermissionScorerTuning.fromJson(Object? scorers) {
    if (scorers is! Map) return PermissionScorerTuning();
    return PermissionScorerTuning(
      maxLower: (scorers['maxLower'] as num?)?.toInt(),
      minConfidence: scorers['minConfidence'],
      skipAtOrBelow: scorers['skipAtOrBelow'],
    );
  }

  /// The stored value, or null (the default) when empty or not 0-9.
  int? get maxLowerValue {
    final v = int.tryParse(maxLower.text.trim());
    return v != null && v >= 0 && v <= 9 ? v : null;
  }

  Map<String, Object?> toJson() => {
    if (maxLowerValue != null) 'maxLower': maxLowerValue,
    if (minConfidence != null) 'minConfidence': minConfidence,
    if (skipAtOrBelow != null) 'skipAtOrBelow': skipAtOrBelow,
  };

  void dispose() => maxLower.dispose();
}

/// Reads `settings.permissions.rules` into editable drafts.
List<PermissionRuleDraft> permissionRuleDrafts(Map<String, dynamic>? settings) {
  final permissions = settings?['permissions'];
  final rules = permissions is Map ? permissions['rules'] : null;
  return [
    if (rules is List)
      for (final r in rules)
        if (r is Map)
          PermissionRuleDraft.fromJson(Map<String, Object?>.from(r)),
  ];
}

/// The `permissions` object of an assistant settings patch: the rule list and
/// the scorer tiers. The permission mode is left as stored.
Map<String, Object?> permissionsPatch({
  required List<PermissionRuleDraft> rules,
  required PermissionScorerDraft fast,
  required PermissionScorerDraft deep,
  PermissionScorerTuning? tuning,
}) {
  final fastJson = fast.toJson();
  final deepJson = deep.toJson();
  return {
    'rules': [
      for (final r in rules)
        if (r.match.text.trim().isNotEmpty) r.toJson(),
    ],
    'scorers': fastJson == null && deepJson == null
        ? null
        : {'fast': fastJson, 'deep': deepJson, ...?tuning?.toJson()},
  };
}

/// Editor for an assistant's permission rules: one dense row per rule with the
/// tool, the pattern and what happens when a call matches.
class PermissionRulesEditor extends StatelessWidget {
  const PermissionRulesEditor({
    super.key,
    required this.rules,
    required this.onChanged,
  });

  final List<PermissionRuleDraft> rules;

  /// Called after a row is added, removed or edited.
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          'A call that matches a rule gets its score. Turn on Ask the scorers '
          'to let the fast and deep scorers (if set up) adjust it. Commands match word by '
          'word (git push *); files match by path (**/.env*). A built-in '
          'refusal cannot be lowered.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        for (var i = 0; i < rules.length; i++)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: _RuleRow(
              key: ValueKey(rules[i]),
              index: i,
              rule: rules[i],
              onChanged: onChanged,
              onRemove: () {
                rules.removeAt(i).dispose();
                onChanged();
              },
            ),
          ),
        Align(
          alignment: Alignment.centerLeft,
          child: TextButton.icon(
            key: const Key('permission-rule-add'),
            onPressed: () {
              rules.add(PermissionRuleDraft());
              onChanged();
            },
            icon: const Icon(Icons.add, size: 18),
            label: const Text('Add rule'),
          ),
        ),
      ],
    );
  }
}

class _RuleRow extends StatelessWidget {
  const _RuleRow({
    super.key,
    required this.index,
    required this.rule,
    required this.onChanged,
    required this.onRemove,
  });

  final int index;
  final PermissionRuleDraft rule;
  final VoidCallback onChanged;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final fields = Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 168,
          child: DropdownButtonFormField<String>(
            key: Key('permission-rule-tool-$index'),
            initialValue: permissionRuleTools.contains(rule.tool)
                ? rule.tool
                : '*',
            isExpanded: true,
            decoration: const InputDecoration(
              labelText: 'Tool',
              border: OutlineInputBorder(),
              isDense: true,
            ),
            items: [
              for (final t in permissionRuleTools)
                DropdownMenuItem(
                  value: t,
                  child: Text(t == '*' ? 'Any tool' : t),
                ),
            ],
            onChanged: (v) {
              if (v != null) {
                rule.tool = v;
                onChanged();
              }
            },
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: TextField(
            key: Key('permission-rule-match-$index'),
            controller: rule.match,
            style: const TextStyle(fontFamily: 'JetBrains Mono', fontSize: 13),
            decoration: InputDecoration(
              labelText: 'Pattern',
              hintText: rule.tool == 'run_command' ? 'git push *' : '**/.env*',
              border: const OutlineInputBorder(),
              isDense: true,
            ),
            onChanged: (_) => onChanged(),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 144,
          child: PermissionScoreDropdown(
            fieldKey: Key('permission-rule-score-$index'),
            value: rule.risk,
            onChanged: (v) {
              rule.risk = v;
              onChanged();
            },
          ),
        ),
        IconButton(
          key: Key('permission-rule-remove-$index'),
          tooltip: 'Remove rule',
          onPressed: onRemove,
          icon: const Icon(Icons.close, size: 18),
        ),
      ],
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        fields,
        AskScorersSwitch(
          switchKey: Key('permission-rule-consult-$index'),
          value: rule.consult,
          onChanged: (v) {
            rule.consult = v;
            onChanged();
          },
        ),
      ],
    );
  }
}

/// The deep scorer's "max lowering" field.
class MaxLowerField extends StatelessWidget {
  const MaxLowerField({
    super.key,
    required this.tuning,
    required this.onChanged,
  });

  final PermissionScorerTuning tuning;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 128,
          child: TextField(
            key: const Key('permission-scorer-max-lower'),
            controller: tuning.maxLower,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: const InputDecoration(
              labelText: 'Max lowering',
              hintText: '2 (default)',
              border: OutlineInputBorder(),
              isDense: true,
            ),
            onChanged: (_) => onChanged(),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            'How many points the deep scorer may lower a rule\'s score, 0-9. '
            '0 never lowers it. Empty uses the default of 2. A session that '
            'has read web content never lowers.',
            style: TextStyle(color: tokens.textSecondary, fontSize: 12),
          ),
        ),
      ],
    );
  }
}

/// Picker for one scorer tier: an inference connection and a model on it.
class PermissionScorerPicker extends StatelessWidget {
  const PermissionScorerPicker({
    super.key,
    required this.label,
    required this.help,
    required this.scorer,
    required this.connections,
    required this.onChanged,
    this.withStrategy = false,
  });

  final String label;
  final String help;
  final PermissionScorerDraft scorer;
  final List<InferenceConnection> connections;
  final VoidCallback onChanged;

  /// The fast tier also picks a System One answer strategy.
  final bool withStrategy;

  /// Free text: the fast tier names a System One model alias, which no
  /// inference connection lists.
  Widget _modelField() {
    return TextField(
      key: Key('permission-scorer-$label-model'),
      controller: scorer.model,
      enabled: scorer.connectionId != null,
      style: const TextStyle(fontFamily: 'JetBrains Mono', fontSize: 13),
      decoration: const InputDecoration(
        labelText: 'Model',
        hintText: 'jev-latest',
        border: OutlineInputBorder(),
        isDense: true,
      ),
      onChanged: (_) => onChanged(),
    );
  }

  /// The deep tier picks from the supported models the chosen connection
  /// lists. A stored model the connection no longer lists, or now flags
  /// unsupported, stays selectable so saving keeps it.
  Widget _modelDropdown() {
    InferenceConnection? connection;
    for (final c in connections) {
      if (c.id == scorer.connectionId) connection = c;
    }
    final models = connection?.models ?? const <ModelInfo>[];
    final current = scorer.model.text.trim();
    final stored =
        current.isNotEmpty &&
        !models.any((m) => m.id == current && !m.unsupported);
    return DropdownButtonFormField<String>(
      key: ValueKey('permission-scorer-$label-model-${scorer.connectionId}'),
      initialValue: current.isEmpty ? null : current,
      isExpanded: true,
      decoration: const InputDecoration(
        labelText: 'Model',
        border: OutlineInputBorder(),
        isDense: true,
      ),
      items: [
        if (stored) DropdownMenuItem(value: current, child: Text(current)),
        for (final m in models)
          if (!m.unsupported)
            DropdownMenuItem(value: m.id, child: Text(m.name)),
      ],
      onChanged: connection == null
          ? null
          : (v) {
              scorer.model.text = v ?? '';
              onChanged();
            },
    );
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final known = connections.any((c) => c.id == scorer.connectionId);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(help, style: TextStyle(color: tokens.textSecondary, fontSize: 12)),
        const SizedBox(height: 8),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: DropdownButtonFormField<String?>(
                key: Key('permission-scorer-$label-connection'),
                initialValue: known ? scorer.connectionId : null,
                isExpanded: true,
                decoration: InputDecoration(
                  labelText: '$label connection',
                  border: const OutlineInputBorder(),
                  isDense: true,
                ),
                items: [
                  const DropdownMenuItem(value: null, child: Text('Not used')),
                  for (final c in connections)
                    DropdownMenuItem(value: c.id, child: Text(c.name)),
                ],
                onChanged: (v) {
                  if (v != scorer.connectionId && !withStrategy) {
                    scorer.model.clear();
                  }
                  scorer.connectionId = v;
                  onChanged();
                },
              ),
            ),
            const SizedBox(width: 8),
            Expanded(child: withStrategy ? _modelField() : _modelDropdown()),
            if (withStrategy) ...[
              const SizedBox(width: 8),
              SizedBox(
                width: 128,
                child: DropdownButtonFormField<String?>(
                  key: Key('permission-scorer-$label-strategy'),
                  initialValue:
                      permissionScorerStrategies.contains(scorer.strategy)
                      ? scorer.strategy
                      : null,
                  isExpanded: true,
                  decoration: const InputDecoration(
                    labelText: 'Strategy',
                    border: OutlineInputBorder(),
                    isDense: true,
                  ),
                  items: [
                    const DropdownMenuItem(value: null, child: Text('Default')),
                    for (final s in permissionScorerStrategies)
                      DropdownMenuItem(value: s, child: Text(s)),
                  ],
                  onChanged: scorer.connectionId == null
                      ? null
                      : (v) {
                          scorer.strategy = v;
                          onChanged();
                        },
                ),
              ),
            ],
          ],
        ),
      ],
    );
  }
}
