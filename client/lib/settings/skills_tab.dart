import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';

/// The level a skill toggle is edited at.
enum SkillScope {
  plane('Plane'),
  project('Project'),
  assistant('Assistant');

  SkillScope(this.label);

  final String label;
}

/// Settings tab listing the skills the plane would offer, with where each one
/// comes from, its full text, and an on/off toggle at plane, project or
/// assistant level. Built-in skills are read-only: they can be switched off,
/// not edited. A skill switched off at any level is off.
class SkillsTab extends StatefulWidget {
  const SkillsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<SkillsTab> createState() => _SkillsTabState();
}

class _SkillsTabState extends State<SkillsTab> {
  SkillScope _scope = SkillScope.plane;
  PlaneSettings _plane = const PlaneSettings();
  List<Project> _projects = const [];
  List<Assistant> _assistants = const [];
  String? _projectId;
  String? _assistantId;
  SkillList _list = const SkillList();
  bool _loading = true;
  bool _busy = false;
  OperatorFailure? _failure;
  String? _error;

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('skills load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _failure = null;
    });
    try {
      final plane = await widget.catalog.getSettings();
      final projects = await widget.catalog.listProjects();
      final assistants = await widget.catalog.listAssistants();
      if (!mounted) return;
      _plane = plane;
      _projects = projects;
      _assistants = assistants;
      _projectId ??= projects.isEmpty ? null : projects.first.id;
      _assistantId ??= assistants.isEmpty ? null : assistants.first.id;
      await _refresh();
    } on Object catch (e, s) {
      AppLog.record('skills load failed: $e', s);
      if (!mounted) return;
      setState(() {
        _loading = false;
        _failure = operatorFailureFrom(e);
      });
    }
  }

  Future<void> _refresh() async {
    final list = await widget.catalog.listSkills(
      projectId: _scope == SkillScope.project ? _projectId : null,
      assistantId: _scope == SkillScope.assistant ? _assistantId : null,
    );
    if (!mounted) return;
    setState(() {
      _list = list;
      _loading = false;
      _error = null;
    });
  }

  /// The names switched off at the selected level.
  Set<String> get _disabledHere => switch (_scope) {
    SkillScope.plane => SkillList.disabledIn(_plane.skills),
    SkillScope.project => SkillList.disabledIn(
      _projects
          .where((p) => p.id == _projectId)
          .firstOrNull
          ?.settings['skills'],
    ),
    SkillScope.assistant => SkillList.disabledIn(
      _assistants
          .where((a) => a.id == _assistantId)
          .firstOrNull
          ?.settings['skills'],
    ),
  };

  bool get _canToggle => switch (_scope) {
    SkillScope.plane => true,
    SkillScope.project => _projectId != null,
    SkillScope.assistant => _assistantId != null,
  };

  Future<void> _toggle(String name, {required bool on}) async {
    final disabled = {..._disabledHere};
    if (on) {
      disabled.remove(name);
    } else {
      disabled.add(name);
    }
    final skills = {'disabled': (disabled.toList()..sort())};
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      switch (_scope) {
        case SkillScope.plane:
          _plane = await widget.catalog.patchSettings(skills: skills);
        case SkillScope.project:
          final updated = await widget.catalog.updateProject(
            _projectId!,
            settings: {'skills': skills},
          );
          _projects = [
            for (final p in _projects)
              if (p.id == updated.id) updated else p,
          ];
        case SkillScope.assistant:
          final updated = await widget.catalog.updateAssistant(
            _assistantId!,
            settings: {'skills': skills},
          );
          _assistants = [
            for (final a in _assistants)
              if (a.id == updated.id) updated else a,
          ];
      }
      await _refresh();
    } on Object catch (e, s) {
      AppLog.record('skills toggle failed: $e', s);
      if (!mounted) return;
      setState(() => _error = operatorMessageFor(operatorFailureFrom(e)));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _showText(SkillEntry entry) async {
    SkillEntry full;
    try {
      full = await widget.catalog.getSkill(
        entry.name,
        projectId: _scope == SkillScope.project ? _projectId : null,
        assistantId: _scope == SkillScope.assistant ? _assistantId : null,
      );
    } on Object catch (e, s) {
      AppLog.record('skill text failed: $e', s);
      if (!mounted) return;
      setState(() => _error = operatorMessageFor(operatorFailureFrom(e)));
      return;
    }
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('/${full.name}'),
        content: SizedBox(
          width: 640,
          child: SingleChildScrollView(
            child: SelectableText(
              full.body,
              key: const Key('skill-text'),
              style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  void _setScope(SkillScope scope) {
    if (scope == _scope) return;
    setState(() {
      _scope = scope;
      _loading = true;
    });
    unawaited(
      _refresh().catchError((Object e, StackTrace s) {
        AppLog.record('skills refresh: $e', s);
        if (mounted) {
          setState(() {
            _loading = false;
            _error = operatorMessageFor(operatorFailureFrom(e));
          });
        }
      }),
    );
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    if (_failure case final failure?) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(operatorMessageFor(failure), textAlign: TextAlign.center),
            const SizedBox(height: 16),
            FilledButton(onPressed: _startLoad, child: const Text('Retry')),
          ],
        ),
      );
    }
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        Text(
          'Skills are folders of instructions the assistant can load. A skill '
          'switched off at any level is off. Built-in skills are read-only; '
          'to change one, copy it into a project\'s .agent-fabric/skills '
          'folder.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 12,
          runSpacing: 8,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            SegmentedButton<SkillScope>(
              key: const Key('skills-scope'),
              segments: [
                for (final s in SkillScope.values)
                  ButtonSegment(value: s, label: Text(s.label)),
              ],
              selected: {_scope},
              onSelectionChanged: (s) => _setScope(s.first),
            ),
            if (_scope == SkillScope.project)
              _picker(
                key: const Key('skills-project'),
                label: 'Project',
                value: _projectId,
                items: {for (final p in _projects) p.id: p.name},
                onChanged: (id) {
                  _projectId = id;
                  setState(() => _loading = true);
                  unawaited(_refresh());
                },
              ),
            if (_scope == SkillScope.assistant)
              _picker(
                key: const Key('skills-assistant'),
                label: 'Assistant',
                value: _assistantId,
                items: {for (final a in _assistants) a.id: a.name},
                onChanged: (id) {
                  _assistantId = id;
                  setState(() => _loading = true);
                  unawaited(_refresh());
                },
              ),
          ],
        ),
        const SizedBox(height: 4),
        Text(switch (_scope) {
          SkillScope.plane =>
            'Plane level: applies to every assistant and project. Project '
                'skills are listed under Project.',
          SkillScope.project =>
            'Project level: this project\'s skill files are included.',
          SkillScope.assistant =>
            'Assistant level: applies to this assistant only.',
        }, style: TextStyle(color: tokens.textMuted, fontSize: 12)),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(_error!, style: TextStyle(color: tokens.error)),
          ),
        const SizedBox(height: 8),
        if (_loading)
          const Padding(
            padding: EdgeInsets.all(24),
            child: Center(child: CircularProgressIndicator()),
          )
        else if (_list.skills.isEmpty)
          Padding(
            padding: const EdgeInsets.all(16),
            child: Text(
              'No skills found.',
              key: const Key('skills-empty'),
              style: TextStyle(color: tokens.textMuted),
            ),
          )
        else
          for (final entry in _list.skills) _row(entry, tokens),
        if (_list.problems.isNotEmpty) ...[
          const SizedBox(height: 16),
          Text(
            'Skipped skill folders',
            style: Theme.of(context).textTheme.titleSmall,
          ),
          for (final p in _list.problems)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(
                '${p.path}: ${p.error}',
                style: TextStyle(color: tokens.warning, fontSize: 12),
              ),
            ),
        ],
      ],
    );
  }

  Widget _picker({
    required Key key,
    required String label,
    required String? value,
    required Map<String, String> items,
    required ValueChanged<String?> onChanged,
  }) {
    return SizedBox(
      width: 240,
      child: DropdownButtonFormField<String>(
        key: key,
        initialValue: value,
        isExpanded: true,
        decoration: InputDecoration(
          labelText: label,
          border: const OutlineInputBorder(),
          isDense: true,
        ),
        items: [
          for (final e in items.entries)
            DropdownMenuItem(
              value: e.key,
              child: Text(e.value, overflow: TextOverflow.ellipsis),
            ),
        ],
        onChanged: onChanged,
      ),
    );
  }

  Widget _row(SkillEntry entry, DesignTokens tokens) {
    final onHere = !_disabledHere.contains(entry.name);
    final details = [
      if (entry.requires == 'sandbox') 'needs a sandbox',
      if (entry.maxToolRounds > 0) 'up to ${entry.maxToolRounds} tool rounds',
      if (entry.flags.isNotEmpty)
        'flags: ${entry.flags.map((f) => '--$f').join(' ')}',
      if (entry.overrides.isNotEmpty) 'replaces ${entry.overrides.join(', ')}',
    ];
    return Card(
      key: Key('skill-${entry.name}'),
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        onTap: () => unawaited(_showText(entry)),
        title: Row(
          children: [
            Flexible(
              child: Text(
                '/${entry.name}',
                style: const TextStyle(fontWeight: FontWeight.w600),
                overflow: TextOverflow.ellipsis,
              ),
            ),
            const SizedBox(width: 8),
            Chip(
              key: Key('skill-source-${entry.name}'),
              label: Text(entry.source),
              visualDensity: VisualDensity.compact,
            ),
            if (!entry.enabled) ...[
              const SizedBox(width: 8),
              Text(
                'Off',
                key: Key('skill-off-${entry.name}'),
                style: TextStyle(color: tokens.warning, fontSize: 12),
              ),
            ],
          ],
        ),
        subtitle: Text(
          [
            entry.description,
            if (details.isNotEmpty) details.join(' · '),
          ].join('\n'),
        ),
        isThreeLine: details.isNotEmpty,
        trailing: Semantics(
          label: 'Enable ${entry.name}',
          child: Switch(
            key: Key('skill-toggle-${entry.name}'),
            value: onHere,
            onChanged: _busy || !_canToggle
                ? null
                : (on) => unawaited(_toggle(entry.name, on: on)),
          ),
        ),
      ),
    );
  }
}
