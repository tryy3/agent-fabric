import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/permission_tiers.dart';
import '../ui/theme/design_tokens.dart';
import 'permissions_editor.dart';

/// Settings tab for the plane-wide tool gate: the user's permission rules and
/// the built-in rule tiers with their overrides. Applies to every assistant;
/// an assistant's own rules are added on top.
class PermissionsTab extends StatefulWidget {
  const PermissionsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<PermissionsTab> createState() => _PermissionsTabState();
}

class _PermissionsTabState extends State<PermissionsTab> {
  OperatorFailure? _failure;
  bool _loading = true;
  bool _saving = false;
  bool _dirty = false;
  String? _error;
  List<PermissionRuleDraft> _rules = [];
  List<PermissionTierDraft> _tiers = [];

  /// Stored scorer settings, sent back unchanged: a plane permissions patch
  /// replaces the whole object.
  Object? _scorers;

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  @override
  void dispose() {
    for (final rule in _rules) {
      rule.dispose();
    }
    super.dispose();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('permissions load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _failure = null;
    });
    try {
      final settings = await widget.catalog.getSettings();
      final tiers = await widget.catalog.listPermissionBuiltins();
      if (!mounted) return;
      _apply(settings.permissions, tiers);
    } on Object catch (e, s) {
      AppLog.record('permissions load failed: $e', s);
      if (!mounted) return;
      setState(() {
        _loading = false;
        _failure = operatorFailureFrom(e);
      });
    }
  }

  void _apply(Map<String, dynamic> permissions, List<PermissionTier> tiers) {
    final overrides = permissions['builtins'];
    for (final rule in _rules) {
      rule.dispose();
    }
    setState(() {
      _loading = false;
      _dirty = false;
      _error = null;
      _scorers = permissions['scorers'];
      _rules = permissionRuleDrafts({'permissions': permissions});
      _tiers = [
        for (final tier in tiers)
          PermissionTierDraft(
            tier,
            overrides is Map && overrides[tier.id] is Map
                ? Map<String, Object?>.from(overrides[tier.id] as Map)
                : null,
          ),
      ];
    });
  }

  void _changed() => setState(() => _dirty = true);

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final updated = await widget.catalog.patchSettings(
        permissions: {
          'rules': [
            for (final r in _rules)
              if (r.match.text.trim().isNotEmpty) r.toJson(),
          ],
          'builtins': {
            for (final t in _tiers)
              if (t.toJson() case final override?) t.tier.id: override,
          },
          if (_scorers != null) 'scorers': _scorers,
        },
      );
      if (!mounted) return;
      _apply(updated.permissions, [for (final t in _tiers) t.tier]);
    } on Object catch (e, s) {
      AppLog.record('permissions save failed: $e', s);
      if (!mounted) return;
      setState(() => _error = operatorMessageFor(operatorFailureFrom(e)));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_failure case final failure?) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(operatorMessageFor(failure), textAlign: TextAlign.center),
              const SizedBox(height: 16),
              FilledButton(onPressed: _startLoad, child: const Text('Retry')),
            ],
          ),
        ),
      );
    }
    final tokens = designTokensOf(context);
    final titleStyle = Theme.of(context).textTheme.titleMedium;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text('Your rules', style: titleStyle),
        const SizedBox(height: 4),
        Text(
          'Global default for every assistant. An assistant can add its own '
          'rules in its settings.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        PermissionRulesEditor(rules: _rules, onChanged: _changed),
        const SizedBox(height: 20),
        Text('Built-in rules', style: titleStyle),
        const SizedBox(height: 4),
        Text(
          'What the gate decides on its own, first match wins. The permission '
          'mode turns each score into run, ask or cancel. Your rules above '
          'take precedence, except over refusals.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        for (final tier in _tiers)
          BuiltinTierTile(
            key: Key('builtin-tier-${tier.tier.id}'),
            draft: tier,
            onChanged: _changed,
          ),
        const SizedBox(height: 12),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(_error!, style: TextStyle(color: tokens.error)),
          ),
        Align(
          alignment: Alignment.centerRight,
          child: FilledButton(
            key: const Key('permissions-save'),
            onPressed: _dirty && !_saving ? _save : null,
            child: Text(_saving ? 'Saving…' : 'Save'),
          ),
        ),
      ],
    );
  }
}

/// One built-in rule tier: what it catches and scores, and, expanded, the
/// controls to change its score, scorer use and program list.
class BuiltinTierTile extends StatefulWidget {
  const BuiltinTierTile({
    super.key,
    required this.draft,
    required this.onChanged,
  });

  final PermissionTierDraft draft;
  final VoidCallback onChanged;

  @override
  State<BuiltinTierTile> createState() => _BuiltinTierTileState();
}

class _BuiltinTierTileState extends State<BuiltinTierTile> {
  final _program = TextEditingController();
  String? _programError;

  @override
  void dispose() {
    _program.dispose();
    super.dispose();
  }

  void _update(VoidCallback change) {
    setState(change);
    widget.onChanged();
  }

  void _addProgram() {
    final ok = widget.draft.addProgram(_program.text);
    setState(() {
      _programError = ok ? null : 'Enter one program name, without a path';
    });
    if (ok) {
      _program.clear();
      widget.onChanged();
    }
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final draft = widget.draft;
    final tier = draft.tier;
    final actionLabel = switch (tier.action) {
      'deny' => 'refuses',
      'allow' => 'runs',
      _ => 'asks',
    };
    final summary = [
      actionLabel,
      'risk ${draft.effectiveRisk}',
      if (tier.locked) 'locked',
      if (draft.isModified) 'modified',
    ].join(' · ');
    return ExpansionTile(
      tilePadding: EdgeInsets.zero,
      childrenPadding: const EdgeInsets.only(bottom: 12),
      expandedCrossAxisAlignment: CrossAxisAlignment.start,
      title: Text(tier.title),
      subtitle: Text(
        summary,
        style: TextStyle(
          color: draft.isModified ? tokens.warning : tokens.textMuted,
          fontSize: 12,
        ),
      ),
      children: [
        Text(
          tier.description,
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        if (!tier.locked) ...[
          const SizedBox(height: 8),
          Wrap(
            spacing: 16,
            runSpacing: 8,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              if (tier.action != 'deny')
                SizedBox(
                  width: 128,
                  child: DropdownButtonFormField<int>(
                    key: Key('builtin-risk-${tier.id}'),
                    initialValue: draft.effectiveRisk,
                    decoration: const InputDecoration(
                      labelText: 'Base risk',
                      border: OutlineInputBorder(),
                      isDense: true,
                    ),
                    items: [
                      for (var r = 1; r <= 10; r++)
                        DropdownMenuItem(
                          value: r,
                          child: Text(r == tier.risk ? '$r (default)' : '$r'),
                        ),
                    ],
                    onChanged: (v) {
                      if (v != null) _update(() => draft.setRisk(v));
                    },
                  ),
                ),
              if (tier.action != 'deny')
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Switch(
                      key: Key('builtin-consult-${tier.id}'),
                      value: draft.effectiveConsult,
                      onChanged: (v) =>
                          _update(() => draft.setConsult(value: v)),
                    ),
                    const SizedBox(width: 4),
                    const Text('Ask the scorers'),
                  ],
                ),
              if (draft.isModified)
                TextButton(
                  key: Key('builtin-reset-${tier.id}'),
                  onPressed: () => _update(draft.reset),
                  child: const Text('Reset to default'),
                ),
            ],
          ),
          if (tier.programs != null) ...[
            const SizedBox(height: 10),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                for (final program in draft.programs)
                  InputChip(
                    key: Key('builtin-program-${tier.id}-$program'),
                    label: Text(
                      program,
                      style: const TextStyle(
                        fontFamily: 'JetBrains Mono',
                        fontSize: 12,
                      ),
                    ),
                    visualDensity: VisualDensity.compact,
                    deleteButtonTooltipMessage: 'Remove $program',
                    onDeleted: () =>
                        _update(() => draft.removeProgram(program)),
                  ),
              ],
            ),
            const SizedBox(height: 8),
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SizedBox(
                  width: 240,
                  child: TextField(
                    key: Key('builtin-add-program-${tier.id}'),
                    controller: _program,
                    style: const TextStyle(
                      fontFamily: 'JetBrains Mono',
                      fontSize: 13,
                    ),
                    decoration: InputDecoration(
                      labelText: 'Add program',
                      errorText: _programError,
                      border: const OutlineInputBorder(),
                      isDense: true,
                    ),
                    onSubmitted: (_) => _addProgram(),
                  ),
                ),
                const SizedBox(width: 8),
                TextButton(
                  key: Key('builtin-add-program-button-${tier.id}'),
                  onPressed: _addProgram,
                  child: const Text('Add'),
                ),
              ],
            ),
          ],
        ],
      ],
    );
  }
}
