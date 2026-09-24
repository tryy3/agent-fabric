import 'dart:async';
import 'dart:math';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:agent_fabric_client/core/settings_load_state.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import 'environment_tab.dart';

String newRemoteID() => _newPrefixedID('rmt_');

String newContextItemID() => _newPrefixedID('ctx_');

String _newPrefixedID(String prefix) {
  final rng = Random.secure();
  final bytes = List<int>.generate(8, (_) => rng.nextInt(256));
  final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  return '$prefix$hex';
}

class ProjectsTab extends StatefulWidget {
  const ProjectsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<ProjectsTab> createState() => _ProjectsTabState();
}

class _ProjectsTabState extends State<ProjectsTab> {
  SettingsLoadState<Project> _state = const SettingsLoading();

  @override
  void initState() {
    super.initState();
    _startReload();
  }

  void _startReload() {
    unawaited(
      _reload().catchError((Object e, StackTrace s) {
        AppLog.record('projects reload: $e', s);
      }),
    );
  }

  void _onEditProject(Project project) {
    unawaited(
      _openEditor(project).catchError((Object e, StackTrace s) {
        AppLog.record('projects edit: $e', s);
      }),
    );
  }

  void _onDeleteProject(Project project) {
    unawaited(
      _confirmDelete(project).catchError((Object e, StackTrace s) {
        AppLog.record('projects delete: $e', s);
      }),
    );
  }

  Future<void> _reload() async {
    setState(() => _state = const SettingsLoading());
    try {
      final projects = await widget.catalog.listProjects();
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsReady(projects));
    } on Object catch (e, s) {
      AppLog.record('projects reload failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsFailed(operatorFailureFrom(e)));
    }
  }

  Future<void> _openEditor(Project project) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) =>
          _ProjectEditorDialog(catalog: widget.catalog, project: project),
    );
    if (saved == true) {
      await _reload();
    }
  }

  Future<void> _confirmDelete(Project project) async {
    try {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) {
          return AlertDialog(
            title: const Text('Delete project?'),
            content: Text(
              'Delete ${project.name}? This deletes the project, its settings, and its threads. Workspace files and the linked environment are left in place.',
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(context).pop(false),
                child: const Text('Cancel'),
              ),
              TextButton(
                onPressed: () => Navigator.of(context).pop(true),
                child: const Text('Delete'),
              ),
            ],
          );
        },
      );
      if (confirmed != true) {
        return;
      }
      await widget.catalog.deleteProject(project.id);
      await _reload();
    } on Object catch (e, s) {
      AppLog.record('projects delete failed: $e', s);
      if (!mounted) {
        return;
      }
      _showActionError(e);
    }
  }

  void _showActionError(Object error) {
    final messenger = ScaffoldMessenger.maybeOf(context);
    messenger?.showSnackBar(
      SnackBar(content: Text(operatorMessageFromError(error))),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SettingsLoadBody<Project>(
        state: _state,
        emptyLabel: 'No projects',
        onRetry: _startReload,
        itemBuilder: (context, project) {
          return Semantics(
            button: true,
            label: 'Project ${project.name}',
            child: ListTile(
              key: Key('project-${project.id}'),
              title: Text(project.name),
              subtitle: project.description.isEmpty
                  ? null
                  : Text(project.description),
              trailing: project.name == 'Default'
                  ? null
                  : Semantics(
                      button: true,
                      label: 'Delete project ${project.name}',
                      child: IconButton(
                        key: Key('delete-project-${project.id}'),
                        tooltip: 'Delete project',
                        icon: const Icon(Icons.delete),
                        onPressed: () => _onDeleteProject(project),
                      ),
                    ),
              onTap: () => _onEditProject(project),
            ),
          );
        },
      ),
    );
  }
}

sealed class _EditorLoadState {
  const _EditorLoadState();
}

final class _EditorLoading extends _EditorLoadState {
  const _EditorLoading();
}

final class _EditorFailed extends _EditorLoadState {
  const _EditorFailed(this.failure);

  final OperatorFailure failure;
}

final class _EditorReady extends _EditorLoadState {
  const _EditorReady({
    required this.agents,
    required this.resources,
    required this.resolved,
  });

  final List<Agent> agents;
  final List<Resource> resources;
  final Map<String, dynamic> resolved;
}

class _ProjectEditorDialog extends StatefulWidget {
  const _ProjectEditorDialog({required this.catalog, required this.project});

  final CatalogClient catalog;
  final Project project;

  @override
  State<_ProjectEditorDialog> createState() => _ProjectEditorDialogState();
}

class _RemoteDraft {
  _RemoteDraft({
    required this.id,
    required this.kind,
    required String urlOrBucket,
    required String path,
    required String providerId,
    required this.enabled,
  }) : urlController = TextEditingController(text: urlOrBucket),
       pathController = TextEditingController(text: path),
       providerController = TextEditingController(text: providerId);

  final String id;
  String kind;
  final TextEditingController urlController;
  final TextEditingController pathController;
  final TextEditingController providerController;
  bool enabled;

  void dispose() {
    urlController.dispose();
    pathController.dispose();
    providerController.dispose();
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'kind': kind,
    'urlOrBucket': urlController.text.trim(),
    'path': pathController.text.trim(),
    'providerId': providerController.text.trim(),
    'enabled': enabled,
  };
}

class _ProjectEditorDialogState extends State<_ProjectEditorDialog> {
  late final TextEditingController _name;
  late final TextEditingController _description;
  late final TextEditingController _toolsAllow;
  late final TextEditingController _mcpServers;
  late final TextEditingController _contextItems;
  late final Set<String> _allowedAgents;
  late final List<_RemoteDraft> _remotes;
  final _removedRemoteIDs = <String>{};
  _EditorLoadState _loadState = const _EditorLoading();
  bool _memoryEnabled = false;
  String? _error;

  Map<String, dynamic> get _settings => widget.project.settings;

  Map<String, dynamic> get _environment {
    final raw = _settings['environment'];
    if (raw is Map<String, dynamic>) {
      return Map<String, dynamic>.from(raw);
    }
    if (raw is Map) {
      return Map<String, dynamic>.from(raw);
    }
    return {};
  }

  @override
  void initState() {
    super.initState();
    final project = widget.project;
    _name = TextEditingController(text: project.name);
    _description = TextEditingController(text: project.description);
    final allowed = _settings['allowedAgents'];
    _allowedAgents = {
      if (allowed is List)
        for (final item in allowed)
          if (item is String && item.isNotEmpty) item,
    };
    final tools = _settings['tools'];
    final allow = tools is Map ? tools['allow'] : null;
    _toolsAllow = TextEditingController(
      text: allow is List ? allow.whereType<String>().join(', ') : '',
    );
    final mcp = _settings['mcp'];
    final servers = mcp is Map ? mcp['servers'] : null;
    _mcpServers = TextEditingController(text: _joinNamedStubs(servers));
    final memory = _settings['memory'];
    _memoryEnabled = memory is Map && memory['enabled'] == true;
    final context = _settings['context'];
    final items = context is Map ? context['items'] : null;
    _contextItems = TextEditingController(text: _joinContextURIs(items));
    _remotes = [];
    for (final item in project.remotes) {
      if (item is! Map) {
        continue;
      }
      final map = Map<String, dynamic>.from(item);
      final id = map['id'] as String? ?? '';
      if (id.isEmpty) {
        continue;
      }
      if (map['enabled'] == false) {
        continue;
      }
      _remotes.add(
        _RemoteDraft(
          id: id,
          kind: map['kind'] as String? ?? 'github',
          urlOrBucket: map['urlOrBucket'] as String? ?? '',
          path: map['path'] as String? ?? '',
          providerId: map['providerId'] as String? ?? '',
          enabled: true,
        ),
      );
    }
    _startLoad();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('project editor load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() => _loadState = const _EditorLoading());
    try {
      final agents = await widget.catalog.listAgents();
      final resources = await widget.catalog.listResources();
      Map<String, dynamic> resolved = const {};
      try {
        resolved = await widget.catalog.resolvedEnvironment(widget.project.id);
      } on Object catch (e, s) {
        AppLog.record('resolvedEnvironment: $e', s);
      }
      if (!mounted) {
        return;
      }
      setState(() {
        _loadState = _EditorReady(
          agents: agents,
          resources: resources,
          resolved: resolved,
        );
      });
    } on Object catch (e, s) {
      AppLog.record('project editor load failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _loadState = _EditorFailed(operatorFailureFrom(e)));
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    _toolsAllow.dispose();
    _mcpServers.dispose();
    _contextItems.dispose();
    for (final remote in _remotes) {
      remote.dispose();
    }
    super.dispose();
  }

  void _onSaveIdentity() {
    unawaited(
      _saveIdentity().catchError((Object e, StackTrace s) {
        AppLog.record('project save: $e', s);
      }),
    );
  }

  Future<void> _saveIdentity() async {
    setState(() {
      _error = null;
    });
    try {
      await widget.catalog.updateProject(
        widget.project.id,
        name: _name.text.trim(),
        description: _description.text.trim(),
        settings: {
          'allowedAgents': _allowedAgents.toList()..sort(),
          'tools': {
            'allow': [
              for (final name in _toolsAllow.text.split(','))
                if (name.trim().isNotEmpty) name.trim(),
            ],
          },
          'mcp': {
            'servers': [
              for (final name in _mcpServers.text.split(','))
                if (name.trim().isNotEmpty) {'name': name.trim()},
            ],
          },
          'memory': {'enabled': _memoryEnabled},
          'context': {
            'items': [
              for (final uri in _contextItems.text.split(','))
                if (uri.trim().isNotEmpty)
                  {'id': newContextItemID(), 'kind': 'url', 'uri': uri.trim()},
            ],
          },
        },
        remotes: [
          for (final remote in _remotes) remote.toJson(),
          for (final id in _removedRemoteIDs) {'id': id, 'enabled': false},
        ],
      );
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(true);
    } on Object catch (e, s) {
      AppLog.record('project save failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _error = operatorMessageFromError(e);
      });
    }
  }

  String _joinNamedStubs(dynamic raw) {
    if (raw is! List) {
      return '';
    }
    return [
      for (final item in raw)
        if (item is Map && (item['name'] as String? ?? '').trim().isNotEmpty)
          (item['name'] as String).trim()
        else if (item is String && item.trim().isNotEmpty)
          item.trim(),
    ].join(', ');
  }

  String _joinContextURIs(dynamic raw) {
    if (raw is! List) {
      return '';
    }
    return [
      for (final item in raw)
        if (item is Map && (item['uri'] as String? ?? '').trim().isNotEmpty)
          (item['uri'] as String).trim()
        else if (item is String && item.trim().isNotEmpty)
          item.trim(),
    ].join(', ');
  }

  String _resolvedResourceLabel(Map<String, dynamic> resolved) {
    final resource = resolved['resource'];
    if (resource is Map) {
      final name = resource['name'];
      if (name is String && name.isNotEmpty) {
        return name;
      }
    }
    return 'no resource';
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.project.name),
      content: SizedBox(
        width: 720,
        child: switch (_loadState) {
          _EditorLoading() => const SizedBox(
            height: 160,
            child: Center(child: CircularProgressIndicator()),
          ),
          _EditorFailed(:final failure) => Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(operatorMessageFor(failure), textAlign: TextAlign.center),
                const SizedBox(height: 16),
                Semantics(
                  button: true,
                  label: 'Retry',
                  child: FilledButton(
                    onPressed: _startLoad,
                    child: const Text('Retry'),
                  ),
                ),
              ],
            ),
          ),
          _EditorReady(:final agents, :final resources, :final resolved) =>
            SingleChildScrollView(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  TextField(
                    key: const Key('project-name'),
                    controller: _name,
                    decoration: const InputDecoration(labelText: 'Name'),
                  ),
                  TextField(
                    key: const Key('project-description'),
                    controller: _description,
                    decoration: const InputDecoration(labelText: 'Description'),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    'Resolved environment',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  Text(
                    _resolvedResourceLabel(resolved),
                    key: const Key('project-resolved-environment'),
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 12),
                  EnvironmentEditor(
                    resources: resources,
                    initial: _environment,
                    keyPrefix: 'project',
                    emptyChoiceLabel: 'Use global default',
                    saveKeyName: 'project-environment-save',
                    saveLabel: 'Save environment',
                    embedded: true,
                    onSave: (environment) async {
                      await widget.catalog.updateProject(
                        widget.project.id,
                        settings: {'environment': environment},
                      );
                    },
                  ),
                  const SizedBox(height: 16),
                  Text(
                    'Allowed agents',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  Text(
                    'Empty means every agent may run in this project.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  for (final agent in agents)
                    CheckboxListTile(
                      key: Key('project-agent-${agent.id}'),
                      contentPadding: EdgeInsets.zero,
                      title: Text(agent.name),
                      value: _allowedAgents.contains(agent.id),
                      onChanged: (checked) {
                        setState(() {
                          if (checked == true) {
                            _allowedAgents.add(agent.id);
                          } else {
                            _allowedAgents.remove(agent.id);
                          }
                        });
                      },
                    ),
                  TextField(
                    key: const Key('project-tools-allow'),
                    controller: _toolsAllow,
                    decoration: const InputDecoration(
                      labelText: 'Tool allow-list',
                      helperText: 'Comma-separated. Stored only; tools are not filtered yet.',
                    ),
                  ),
                  const SizedBox(height: 16),
                  TextField(
                    key: const Key('project-mcp-servers'),
                    controller: _mcpServers,
                    decoration: const InputDecoration(
                      labelText: 'MCP extras',
                      helperText: 'Coming soon - extra MCP servers for this workspace are stored, not executed.',
                    ),
                  ),
                  SwitchListTile(
                    key: const Key('project-memory-enabled'),
                    contentPadding: EdgeInsets.zero,
                    title: const Text('Memory'),
                    subtitle: const Text(
                      'Coming soon - project memory records are not retrieved yet.',
                    ),
                    value: _memoryEnabled,
                    onChanged: (enabled) {
                      setState(() {
                        _memoryEnabled = enabled;
                      });
                    },
                  ),
                  TextField(
                    key: const Key('project-context-items'),
                    controller: _contextItems,
                    decoration: const InputDecoration(
                      labelText: 'Context URLs',
                      helperText: 'Coming soon - repos, docs, and URLs hydrate in a later slice.',
                    ),
                  ),
                  const SizedBox(height: 16),
                  Text(
                    'Remotes',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  Text(
                    'GitHub and S3 remotes are stubs. Tokens stay on Providers.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 8),
                  for (final remote in _remotes) ...[
                    _RemoteCard(
                      remote: remote,
                      onChanged: () => setState(() {}),
                      onRemove: () {
                        setState(() {
                          _remotes.remove(remote);
                          _removedRemoteIDs.add(remote.id);
                          remote.dispose();
                        });
                      },
                    ),
                    const SizedBox(height: 8),
                  ],
                  OutlinedButton(
                    key: const Key('project-remote-add'),
                    onPressed: () {
                      setState(() {
                        _remotes.add(
                          _RemoteDraft(
                            id: newRemoteID(),
                            kind: 'github',
                            urlOrBucket: '',
                            path: '',
                            providerId: '',
                            enabled: true,
                          ),
                        );
                      });
                    },
                    child: const Text('Add remote'),
                  ),
                  if (_error != null) Text(_error!),
                ],
              ),
            ),
        },
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(false),
          child: const Text('Close'),
        ),
        TextButton(
          key: const Key('project-save'),
          onPressed: _loadState is _EditorReady ? _onSaveIdentity : null,
          child: const Text('Save'),
        ),
      ],
    );
  }
}

class _RemoteCard extends StatelessWidget {
  const _RemoteCard({
    required this.remote,
    required this.onChanged,
    required this.onRemove,
  });

  final _RemoteDraft remote;
  final VoidCallback onChanged;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    return Card(
      key: Key('project-remote-${remote.id}'),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          children: [
            DropdownButtonFormField<String>(
              key: Key('project-remote-${remote.id}-kind'),
              initialValue: remote.kind,
              decoration: const InputDecoration(labelText: 'Kind'),
              items: const [
                DropdownMenuItem(value: 'github', child: Text('github')),
                DropdownMenuItem(value: 's3', child: Text('s3')),
              ],
              onChanged: (value) {
                if (value != null) {
                  remote.kind = value;
                  onChanged();
                }
              },
            ),
            TextField(
              key: Key('project-remote-${remote.id}-url'),
              controller: remote.urlController,
              decoration: const InputDecoration(labelText: 'URL or bucket'),
            ),
            TextField(
              controller: remote.pathController,
              decoration: const InputDecoration(labelText: 'Path'),
            ),
            TextField(
              controller: remote.providerController,
              decoration: const InputDecoration(
                labelText: 'Provider id',
                helperText: 'Credential lives on Settings -> Providers',
              ),
            ),
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton(
                onPressed: onRemove,
                child: const Text('Remove'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
