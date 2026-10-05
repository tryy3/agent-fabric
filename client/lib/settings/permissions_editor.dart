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

/// Rule actions, most to least restrictive (the control plane applies the
/// most restrictive matching rule).
const permissionRuleActions = <String>['deny', 'ask', 'allow'];

/// System One answer strategies for the fast scorer.
const permissionScorerStrategies = <String>['score', 'bands'];

/// One editable permission rule row.
class PermissionRuleDraft {
  PermissionRuleDraft({
    this.tool = 'run_command',
    String match = '',
    this.action = 'ask',
    this.risk,
  }) : match = TextEditingController(text: match);

  String tool;
  final TextEditingController match;
  String action;

  /// Preserved from stored rules; not editable here.
  final int? risk;

  factory PermissionRuleDraft.fromJson(Map<String, Object?> json) {
    return PermissionRuleDraft(
      tool: json['tool'] as String? ?? 'run_command',
      match: json['match'] as String? ?? '',
      action: json['action'] as String? ?? 'ask',
      risk: (json['risk'] as num?)?.toInt(),
    );
  }

  Map<String, Object?> toJson() => {
    'tool': tool,
    'match': match.text.trim(),
    'action': action,
    if (risk != null) 'risk': risk,
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
        : {'fast': fastJson, 'deep': deepJson},
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
          'Calls that match a rule always run, always ask, or are refused, '
          'in every permission mode. Commands match word by word '
          '(git push *); files match by path (**/.env*). '
          'Built-in refusals cannot be allowed.',
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
    final tokens = designTokensOf(context);
    return Row(
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
          width: 104,
          child: DropdownButtonFormField<String>(
            key: Key('permission-rule-action-$index'),
            initialValue: rule.action,
            isExpanded: true,
            decoration: const InputDecoration(
              labelText: 'Action',
              border: OutlineInputBorder(),
              isDense: true,
            ),
            items: [
              for (final a in permissionRuleActions)
                DropdownMenuItem(
                  value: a,
                  child: Text(
                    switch (a) {
                      'deny' => 'Deny',
                      'ask' => 'Ask',
                      _ => 'Allow',
                    },
                    style: TextStyle(
                      color: a == 'deny' ? tokens.error : tokens.textPrimary,
                    ),
                  ),
                ),
            ],
            onChanged: (v) {
              if (v != null) {
                rule.action = v;
                onChanged();
              }
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
                  scorer.connectionId = v;
                  onChanged();
                },
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: TextField(
                key: Key('permission-scorer-$label-model'),
                controller: scorer.model,
                enabled: scorer.connectionId != null,
                style: const TextStyle(
                  fontFamily: 'JetBrains Mono',
                  fontSize: 13,
                ),
                decoration: InputDecoration(
                  labelText: 'Model',
                  hintText: withStrategy ? 'jev-latest' : null,
                  border: const OutlineInputBorder(),
                  isDense: true,
                ),
                onChanged: (_) => onChanged(),
              ),
            ),
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
