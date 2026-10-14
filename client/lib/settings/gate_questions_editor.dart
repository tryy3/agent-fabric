import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/gate_questions.dart';
import '../ui/theme/design_tokens.dart';
import 'permissions_editor.dart';

/// What a gate question is, shown above the editor.
const gateQuestionsHelp =
    'Scorers that use the questions strategy answer these yes/no questions '
    'about each tool call. A tripwire is a question where a yes sets a '
    'minimum score the later tiers cannot lower. Policy questions check the '
    'call against your gate instructions and only run when gate instructions '
    'exist. Changes apply from the next session.';

/// The heading of a question family.
String gateQuestionFamilyLabel(String id) => switch (id) {
  'command' => 'Commands',
  'web' => 'Web',
  'file_read' => 'File reads',
  'file_write' => 'File writes',
  _ => id,
};

/// What is wrong with [question] as a custom question, keyed by field. Empty
/// when it can be saved. [others] are the other custom questions and
/// [builtinIds] the built-in question ids, which ids must not repeat.
Map<String, String> customGateQuestionErrors(
  CustomGateQuestion question, {
  required GateQuestionCatalog catalog,
  required Iterable<CustomGateQuestion> others,
  required Set<String> builtinIds,
}) {
  final limits = catalog.limits;
  final errors = <String, String>{};
  final id = question.id;
  if (!RegExp(limits.idPattern).hasMatch(id)) {
    errors['id'] =
        'Lowercase letters, digits and underscores, starting with a letter '
        '(2-32 characters)';
  } else if (builtinIds.contains(id) || others.any((o) => o.id == id)) {
    errors['id'] = 'Another question already uses this id';
  }
  if (question.tools.isEmpty) {
    errors['tools'] = 'Pick at least one tool';
  } else {
    final touched = catalog.familiesOf(question.tools);
    for (final family in touched) {
      final used = others
          .where((o) => catalog.familiesOf(o.tools).contains(family))
          .length;
      if (used >= limits.maxCustomPerFamily) {
        errors['tools'] =
            'At most ${limits.maxCustomPerFamily} custom questions per tool '
            'family; ${gateQuestionFamilyLabel(family)} is full';
        break;
      }
    }
  }
  final length = question.text.trim().length;
  if (length < limits.minTextChars || length > limits.maxTextChars) {
    errors['text'] = '${limits.minTextChars}-${limits.maxTextChars} characters';
  }
  return errors;
}

/// Editor for `permissions.questions`: every built-in gate question by tool
/// family with its text, score and switch, and the user's own tripwires.
/// Loads the questions and limits from the plane.
class GateQuestionsEditor extends StatefulWidget {
  const GateQuestionsEditor({
    super.key,
    required this.catalog,
    required this.draft,
    required this.onChanged,
  });

  final CatalogClient catalog;
  final GateQuestionsDraft draft;

  /// Called after any edit.
  final VoidCallback onChanged;

  @override
  State<GateQuestionsEditor> createState() => _GateQuestionsEditorState();
}

class _GateQuestionsEditorState extends State<GateQuestionsEditor> {
  GateQuestionCatalog? _loaded;
  OperatorFailure? _failure;

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  void _startLoad() {
    setState(() => _failure = null);
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('gate questions load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    try {
      final loaded = await widget.catalog.listPermissionQuestions();
      if (!mounted) return;
      setState(() => _loaded = loaded);
    } on Object catch (e, s) {
      AppLog.record('gate questions load failed: $e', s);
      if (!mounted) return;
      setState(() => _failure = operatorFailureFrom(e));
    }
  }

  void _update(VoidCallback change) {
    setState(change);
    widget.onChanged();
  }

  Future<void> _editCustom(CustomGateQuestion? existing) async {
    final catalog = _loaded;
    if (catalog == null) return;
    final result = await showDialog<CustomGateQuestion>(
      context: context,
      builder: (_) => _CustomQuestionDialog(
        catalog: catalog,
        existing: existing,
        others: [
          for (final c in widget.draft.custom)
            if (!identical(c, existing)) c,
        ],
        builtinIds: {for (final q in catalog.questions) q.id},
      ),
    );
    if (result == null || !mounted) return;
    _update(() {
      final custom = widget.draft.custom;
      final index = existing == null ? -1 : custom.indexOf(existing);
      if (index < 0) {
        custom.add(result);
      } else {
        custom[index] = result;
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final catalog = _loaded;
    if (_failure case final failure?) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            operatorMessageFor(failure),
            key: const Key('gate-questions-load-error'),
            style: TextStyle(color: tokens.error),
          ),
          TextButton(
            key: const Key('gate-questions-retry'),
            onPressed: _startLoad,
            child: const Text('Retry'),
          ),
        ],
      );
    }
    if (catalog == null) {
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 12),
        child: Center(child: CircularProgressIndicator()),
      );
    }
    final draft = widget.draft;
    final familyIds = {
      for (final f in catalog.families) f.id,
      for (final q in catalog.questions) q.family,
    };
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          gateQuestionsHelp,
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        for (final family in familyIds) ...[
          Padding(
            padding: const EdgeInsets.only(top: 8, bottom: 4),
            child: Row(
              children: [
                Text(
                  gateQuestionFamilyLabel(family),
                  key: Key('gate-family-$family'),
                  style: Theme.of(context).textTheme.titleSmall,
                ),
                const SizedBox(width: 8),
                Flexible(
                  child: Text(
                    catalog.families
                        .firstWhere(
                          (f) => f.id == family,
                          orElse: () => GateQuestionFamily(id: family),
                        )
                        .tools
                        .join(', '),
                    style: TextStyle(
                      color: tokens.textMuted,
                      fontFamily: 'JetBrains Mono',
                      fontSize: 12,
                    ),
                  ),
                ),
              ],
            ),
          ),
          for (final q in catalog.questions.where((q) => q.family == family))
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: _QuestionRow(
                key: Key('gate-question-$family-${q.id}'),
                question: q,
                draft: draft,
                limits: catalog.limits,
                onChanged: () => _update(() {}),
              ),
            ),
        ],
        const SizedBox(height: 12),
        Text('Your tripwires', style: Theme.of(context).textTheme.titleSmall),
        const SizedBox(height: 4),
        Text(
          'Questions you write. A yes sets a minimum score for calls to the '
          'tools you pick. An assistant\'s entry replaces the plane\'s with '
          'the same id, so an assistant can hide a plane question by adding '
          'its id with the switch off.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        for (var i = 0; i < draft.custom.length; i++)
          _CustomRow(
            key: ValueKey(draft.custom[i]),
            index: i,
            question: draft.custom[i],
            onToggle: (v) => _update(() => draft.custom[i].enabled = v),
            onEdit: () => _editCustom(draft.custom[i]),
            onRemove: () => _update(() => draft.custom.removeAt(i)),
          ),
        Align(
          alignment: Alignment.centerLeft,
          child: TextButton.icon(
            key: const Key('gate-custom-add'),
            onPressed: () => _editCustom(null),
            icon: const Icon(Icons.add, size: 18),
            label: const Text('Add tripwire'),
          ),
        ),
      ],
    );
  }
}

/// One built-in question: id and kind, its text, score and switch.
class _QuestionRow extends StatefulWidget {
  const _QuestionRow({
    super.key,
    required this.question,
    required this.draft,
    required this.limits,
    required this.onChanged,
  });

  final GateQuestion question;
  final GateQuestionsDraft draft;
  final GateQuestionLimits limits;
  final VoidCallback onChanged;

  @override
  State<_QuestionRow> createState() => _QuestionRowState();
}

class _QuestionRowState extends State<_QuestionRow> {
  late final TextEditingController _text;
  final _focus = FocusNode();

  GateQuestion get _q => widget.question;

  @override
  void initState() {
    super.initState();
    _text = TextEditingController(text: widget.draft.textOf(_q));
  }

  @override
  void didUpdateWidget(_QuestionRow old) {
    super.didUpdateWidget(old);
    // A policy question is shared by every family, so an edit made in another
    // family's row (or a reset) reaches this field too.
    final text = widget.draft.textOf(_q);
    if (!_focus.hasFocus && _text.text != text) _text.text = text;
  }

  @override
  void dispose() {
    _text.dispose();
    _focus.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final draft = widget.draft;
    final q = _q;
    final modified = draft.isModified(q.id);
    final enabled = draft.enabledOf(q);
    final risk = draft.riskOf(q);
    final length = _text.text.trim().length;
    final invalid =
        length < widget.limits.minTextChars ||
        length > widget.limits.maxTextChars;
    final key = '${q.family}-${q.id}';
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        border: Border.all(color: tokens.border),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 8,
            runSpacing: 4,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              Text(
                q.id,
                style: const TextStyle(
                  fontFamily: 'JetBrains Mono',
                  fontSize: 13,
                ),
              ),
              _KindBadge(
                key: Key('gate-question-kind-$key'),
                policy: q.isPolicy,
              ),
              if (modified)
                Text(
                  'modified',
                  key: Key('gate-question-modified-$key'),
                  style: TextStyle(color: tokens.warning, fontSize: 12),
                ),
              if (q.isPolicy)
                Text(
                  'applies to every tool family',
                  style: TextStyle(color: tokens.textMuted, fontSize: 12),
                ),
            ],
          ),
          const SizedBox(height: 8),
          TextField(
            key: Key('gate-question-text-$key'),
            controller: _text,
            focusNode: _focus,
            minLines: 2,
            maxLines: 6,
            decoration: InputDecoration(
              labelText: 'Question',
              border: const OutlineInputBorder(),
              isDense: true,
              errorText: invalid
                  ? '${widget.limits.minTextChars}-'
                        '${widget.limits.maxTextChars} characters'
                  : null,
            ),
            onChanged: (v) {
              draft.setText(q, v);
              widget.onChanged();
            },
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 16,
            runSpacing: 8,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              if (risk != null)
                SizedBox(
                  width: 176,
                  child: PermissionScoreDropdown(
                    key: ValueKey(risk),
                    fieldKey: Key('gate-question-risk-$key'),
                    value: risk,
                    defaultScore: q.risk,
                    onChanged: (v) {
                      draft.setRisk(q, v);
                      widget.onChanged();
                    },
                  ),
                ),
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Switch(
                    key: Key('gate-question-enabled-$key'),
                    value: enabled,
                    onChanged: (v) {
                      draft.setEnabled(q, value: v);
                      widget.onChanged();
                    },
                  ),
                  const SizedBox(width: 4),
                  const Text('Enabled'),
                ],
              ),
              if (modified)
                TextButton(
                  key: Key('gate-question-reset-$key'),
                  onPressed: () {
                    draft.reset(q.id);
                    _text.text = q.text;
                    widget.onChanged();
                  },
                  child: const Text('Reset to default'),
                ),
            ],
          ),
        ],
      ),
    );
  }
}

/// A tripwire or policy label. The words carry the meaning, the color only
/// repeats it.
class _KindBadge extends StatelessWidget {
  const _KindBadge({super.key, required this.policy});

  final bool policy;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final color = policy ? tokens.textSecondary : tokens.warning;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
      decoration: BoxDecoration(
        border: Border.all(color: color),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        policy ? 'policy' : 'tripwire',
        style: TextStyle(color: color, fontSize: 11),
      ),
    );
  }
}

class _CustomRow extends StatelessWidget {
  const _CustomRow({
    super.key,
    required this.index,
    required this.question,
    required this.onToggle,
    required this.onEdit,
    required this.onRemove,
  });

  final int index;
  final CustomGateQuestion question;
  final ValueChanged<bool> onToggle;
  final VoidCallback onEdit;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Switch(
            key: Key('gate-custom-enabled-$index'),
            value: question.enabled,
            onChanged: onToggle,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  '${question.id} · score ${question.risk}',
                  style: const TextStyle(
                    fontFamily: 'JetBrains Mono',
                    fontSize: 13,
                  ),
                ),
                Text(
                  question.tools.join(', '),
                  style: TextStyle(color: tokens.textMuted, fontSize: 12),
                ),
                Text(question.text, style: const TextStyle(fontSize: 13)),
              ],
            ),
          ),
          IconButton(
            key: Key('gate-custom-edit-$index'),
            tooltip: 'Edit tripwire',
            onPressed: onEdit,
            icon: const Icon(Icons.edit_outlined, size: 18),
          ),
          IconButton(
            key: Key('gate-custom-remove-$index'),
            tooltip: 'Remove tripwire',
            onPressed: onRemove,
            icon: const Icon(Icons.close, size: 18),
          ),
        ],
      ),
    );
  }
}

class _CustomQuestionDialog extends StatefulWidget {
  const _CustomQuestionDialog({
    required this.catalog,
    required this.others,
    required this.builtinIds,
    this.existing,
  });

  final GateQuestionCatalog catalog;
  final List<CustomGateQuestion> others;
  final Set<String> builtinIds;
  final CustomGateQuestion? existing;

  @override
  State<_CustomQuestionDialog> createState() => _CustomQuestionDialogState();
}

class _CustomQuestionDialogState extends State<_CustomQuestionDialog> {
  late final TextEditingController _id;
  late final TextEditingController _text;
  late final Set<String> _tools;
  late int _risk;
  late bool _enabled;
  bool _touched = false;

  @override
  void initState() {
    super.initState();
    final e = widget.existing;
    _id = TextEditingController(text: e?.id ?? '');
    _text = TextEditingController(text: e?.text ?? '');
    _tools = {...?e?.tools};
    _risk = e?.risk ?? permissionRuleDefaultScore;
    _enabled = e?.enabled ?? true;
  }

  @override
  void dispose() {
    _id.dispose();
    _text.dispose();
    super.dispose();
  }

  CustomGateQuestion get _draft => CustomGateQuestion(
    id: _id.text.trim(),
    tools: [
      for (final f in widget.catalog.families)
        for (final t in f.tools)
          if (_tools.contains(t)) t,
      // A stored family id or tool the chips do not list is kept as stored.
      for (final t in widget.existing?.tools ?? const <String>[])
        if (!widget.catalog.families.any((f) => f.tools.contains(t))) t,
    ],
    text: _text.text.trim(),
    risk: _risk,
    enabled: _enabled,
  );

  Map<String, String> get _errors => customGateQuestionErrors(
    _draft,
    catalog: widget.catalog,
    others: widget.others,
    builtinIds: widget.builtinIds,
  );

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final errors = _touched ? _errors : const <String, String>{};
    final limits = widget.catalog.limits;
    return AlertDialog(
      title: Text(widget.existing == null ? 'Add tripwire' : 'Edit tripwire'),
      content: SizedBox(
        width: 480,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextField(
                key: const Key('gate-custom-id'),
                controller: _id,
                enabled: widget.existing == null,
                style: const TextStyle(
                  fontFamily: 'JetBrains Mono',
                  fontSize: 13,
                ),
                decoration: InputDecoration(
                  labelText: 'Id',
                  hintText: 'prod_database',
                  border: const OutlineInputBorder(),
                  isDense: true,
                  errorText: errors['id'],
                ),
                onChanged: (_) => setState(() => _touched = true),
              ),
              const SizedBox(height: 12),
              Text('Tools', style: Theme.of(context).textTheme.labelLarge),
              for (final f in widget.catalog.families)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Wrap(
                    spacing: 6,
                    runSpacing: 4,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      SizedBox(
                        width: 80,
                        child: Text(
                          gateQuestionFamilyLabel(f.id),
                          style: TextStyle(
                            color: tokens.textSecondary,
                            fontSize: 12,
                          ),
                        ),
                      ),
                      for (final t in f.tools)
                        FilterChip(
                          key: Key('gate-custom-tool-$t'),
                          label: Text(
                            t,
                            style: const TextStyle(
                              fontFamily: 'JetBrains Mono',
                              fontSize: 12,
                            ),
                          ),
                          visualDensity: VisualDensity.compact,
                          selected: _tools.contains(t),
                          onSelected: (v) => setState(() {
                            _touched = true;
                            if (v) {
                              _tools.add(t);
                            } else {
                              _tools.remove(t);
                            }
                          }),
                        ),
                    ],
                  ),
                ),
              if (errors['tools'] case final e?)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text(
                    e,
                    key: const Key('gate-custom-tools-error'),
                    style: TextStyle(color: tokens.error, fontSize: 12),
                  ),
                ),
              const SizedBox(height: 12),
              TextField(
                key: const Key('gate-custom-text'),
                controller: _text,
                minLines: 3,
                maxLines: 6,
                decoration: InputDecoration(
                  labelText: 'Question',
                  helperText:
                      'A yes/no question about the call. '
                      '${limits.minTextChars}-${limits.maxTextChars} '
                      'characters.',
                  helperMaxLines: 2,
                  border: const OutlineInputBorder(),
                  isDense: true,
                  errorText: errors['text'],
                ),
                onChanged: (_) => setState(() => _touched = true),
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  SizedBox(
                    width: 176,
                    child: PermissionScoreDropdown(
                      fieldKey: const Key('gate-custom-risk'),
                      value: _risk,
                      onChanged: (v) => setState(() => _risk = v),
                    ),
                  ),
                  const SizedBox(width: 16),
                  Switch(
                    key: const Key('gate-custom-enabled'),
                    value: _enabled,
                    onChanged: (v) => setState(() => _enabled = v),
                  ),
                  const SizedBox(width: 4),
                  const Text('Enabled'),
                ],
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        TextButton(
          key: const Key('gate-custom-save'),
          onPressed: () {
            if (_errors.isNotEmpty) {
              setState(() => _touched = true);
              return;
            }
            Navigator.of(context).pop(_draft);
          },
          child: const Text('Save'),
        ),
      ],
    );
  }
}
