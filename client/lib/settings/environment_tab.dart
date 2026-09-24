import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import 'resources_tab.dart';

class EnvironmentTab extends StatefulWidget {
  const EnvironmentTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<EnvironmentTab> createState() => _EnvironmentTabState();
}

class _EnvironmentTabState extends State<EnvironmentTab> {
  List<Resource> _resources = [];
  Map<String, dynamic> _environment = {};
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final settings = await widget.catalog.getSettings();
      final resources = await widget.catalog.listResources();
      if (!mounted) {
        return;
      }
      setState(() {
        _environment = Map<String, dynamic>.from(settings.environment);
        _resources = resources;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
          ? Padding(padding: const EdgeInsets.all(16), child: Text(_error!))
          : EnvironmentEditor(
              resources: _resources,
              initial: _environment,
              keyPrefix: 'environment',
              emptyChoiceLabel: '',
              onSave: (environment) async {
                final updated = await widget.catalog.patchSettings(
                  environment: environment,
                );
                if (!mounted) {
                  return;
                }
                setState(() {
                  _environment = Map<String, dynamic>.from(updated.environment);
                });
              },
            ),
    );
  }
}

class EnvironmentEditor extends StatefulWidget {
  const EnvironmentEditor({
    super.key,
    required this.resources,
    required this.initial,
    required this.onSave,
    required this.keyPrefix,
    required this.emptyChoiceLabel,
    this.saveKeyName,
    this.saveLabel = 'Save',
    this.embedded = false,
  });

  final List<Resource> resources;
  final Map<String, dynamic> initial;
  final Future<void> Function(Map<String, dynamic> environment) onSave;
  final String keyPrefix;
  final String emptyChoiceLabel;
  final String? saveKeyName;
  final String saveLabel;
  final bool embedded;

  @override
  State<EnvironmentEditor> createState() => EnvironmentEditorState();
}

class _PathDraft {
  _PathDraft({
    required this.id,
    required String path,
    required this.enabled,
    required this.whitelisted,
    required this.read,
    required this.write,
    required this.exec,
  }) : pathController = TextEditingController(text: path);

  final String id;
  final TextEditingController pathController;
  bool enabled;
  bool whitelisted;
  bool read;
  bool write;
  bool exec;

  void dispose() {
    pathController.dispose();
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'path': pathController.text.trim(),
    'enabled': enabled,
    'whitelisted': whitelisted,
    'read': read,
    'write': write,
    'exec': exec,
  };
}

class _GrantDraft {
  _GrantDraft({
    required this.whitelisted,
    required this.read,
    required this.write,
    required this.exec,
  });

  bool whitelisted;
  bool read;
  bool write;
  bool exec;
}

class EnvironmentEditorState extends State<EnvironmentEditor> {
  late final TextEditingController _workspaceRoot;
  late String _resourceId;
  final List<_PathDraft> _paths = [];
  final _removedPathIDs = <String>{};
  final _originalPathIDs = <String>{};
  final Map<String, _GrantDraft> _grants = {};
  late Map<String, dynamic> _baseline;
  String? _error;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _baseline = Map<String, dynamic>.from(widget.initial);
    _workspaceRoot = TextEditingController(
      text: _baseline['workspaceRoot'] as String? ?? '',
    );
    _resourceId = _storedResourceId();
    _replacePaths(_pathsFrom(_baseline['extraPaths']));
    _syncGrants();
  }

  @override
  void dispose() {
    _workspaceRoot.dispose();
    for (final path in _paths) {
      path.dispose();
    }
    super.dispose();
  }

  String _storedResourceId() {
    final raw = _baseline['resourceId'];
    if (raw is! String || raw.isEmpty) {
      return '';
    }
    for (final resource in widget.resources) {
      if (resource.id == raw) {
        return raw;
      }
    }
    return '';
  }

  void _replacePaths(List<_PathDraft> next) {
    for (final path in _paths) {
      path.dispose();
    }
    _paths
      ..clear()
      ..addAll(next);
    _removedPathIDs.clear();
    _originalPathIDs
      ..clear()
      ..addAll(next.map((path) => path.id));
  }

  List<_PathDraft> _pathsFrom(dynamic raw) {
    if (raw is! List) {
      return <_PathDraft>[];
    }
    final out = <_PathDraft>[];
    for (final item in raw) {
      if (item is! Map) {
        continue;
      }
      final map = Map<String, dynamic>.from(item);
      final id = map['id'] as String? ?? '';
      if (id.isEmpty) {
        continue;
      }
      out.add(
        _PathDraft(
          id: id,
          path: map['path'] as String? ?? '',
          enabled: map['enabled'] as bool? ?? true,
          whitelisted: map['whitelisted'] as bool? ?? true,
          read: map['read'] as bool? ?? true,
          write: map['write'] as bool? ?? true,
          exec: map['exec'] as bool? ?? false,
        ),
      );
    }
    return out;
  }

  Resource? _selectedResource() {
    for (final resource in widget.resources) {
      if (resource.id == _resourceId) {
        return resource;
      }
    }
    return null;
  }

  List<Map<String, dynamic>> _volumes() {
    final raw = _selectedResource()?.spec['volumes'];
    if (raw is! List) {
      return const [];
    }
    return [
      for (final item in raw)
        if (item is Map) Map<String, dynamic>.from(item),
    ];
  }

  Map<String, Map<String, dynamic>> _grantIndex(dynamic raw) {
    final stored = <String, Map<String, dynamic>>{};
    if (raw is! List) {
      return stored;
    }
    for (final item in raw) {
      if (item is! Map) {
        continue;
      }
      final map = Map<String, dynamic>.from(item);
      final id = map['volumeId'] as String? ?? '';
      if (id.isNotEmpty) {
        stored[id] = map;
      }
    }
    return stored;
  }

  void _remember(Map<String, dynamic> patch) {
    final next = Map<String, dynamic>.from(_baseline);
    for (final entry in patch.entries) {
      if (entry.key == 'grants' && entry.value is List) {
        next['grants'] = _mergedGrants(next['grants'], entry.value as List);
        continue;
      }
      if (entry.value == null) {
        next.remove(entry.key);
      } else {
        next[entry.key] = entry.value;
      }
    }
    _baseline = next;
  }

  List<Map<String, dynamic>> _mergedGrants(dynamic base, List<dynamic> patch) {
    final index = _grantIndex(base);
    final order = index.keys.toList();
    for (final item in patch) {
      if (item is! Map) {
        continue;
      }
      final map = Map<String, dynamic>.from(item);
      final id = map['volumeId'] as String? ?? '';
      if (id.isEmpty) {
        continue;
      }
      final current = index[id] ?? <String, dynamic>{'volumeId': id};
      if (!order.contains(id)) {
        order.add(id);
      }
      for (final entry in map.entries) {
        if (entry.value == null) {
          current.remove(entry.key);
        } else {
          current[entry.key] = entry.value;
        }
      }
      if (current.length <= 1) {
        index.remove(id);
        order.remove(id);
      } else {
        index[id] = current;
      }
    }
    return [
      for (final id in order)
        if (index[id] != null) index[id]!,
    ];
  }

  void _syncGrants() {
    final stored = _grantIndex(_baseline['grants']);
    _grants.clear();
    for (final volume in _volumes()) {
      final id = volume['id'] as String? ?? '';
      if (id.isEmpty) {
        continue;
      }
      final override = stored[id] ?? const <String, dynamic>{};
      _grants[id] = _GrantDraft(
        whitelisted: _flag(override, volume, 'whitelisted'),
        read: _flag(override, volume, 'read'),
        write: _flag(override, volume, 'write'),
        exec: _flag(override, volume, 'exec'),
      );
    }
  }

  bool _flag(
    Map<String, dynamic> override,
    Map<String, dynamic> volume,
    String key,
  ) {
    final value = override[key] ?? volume[key];
    if (value is bool) {
      return value;
    }
    return true;
  }

  Map<String, dynamic> _patch() {
    final environment = <String, dynamic>{
      'resourceId': _resourceId.isEmpty ? null : _resourceId,
    };
    final root = _workspaceRoot.text.trim();
    if (root.isNotEmpty) {
      environment['workspaceRoot'] = root;
    } else if (_baseline['workspaceRoot'] != null) {
      environment['workspaceRoot'] = null;
    }
    final extraPaths = [
      for (final path in _paths) path.toJson(),
      for (final id in _removedPathIDs) {'id': id, 'enabled': false},
    ];
    if (extraPaths.isNotEmpty) {
      environment['extraPaths'] = extraPaths;
    }
    final grants = _sparseGrants();
    if (grants.isNotEmpty) {
      environment['grants'] = grants;
    }
    return environment;
  }

  List<Map<String, dynamic>> _sparseGrants() {
    if (_resourceId.isEmpty) {
      return const [];
    }
    final stored = _grantIndex(_baseline['grants']);
    final out = <Map<String, dynamic>>[];
    for (final volume in _volumes()) {
      final id = volume['id'] as String? ?? '';
      final draft = _grants[id];
      if (id.isEmpty || draft == null) {
        continue;
      }
      final row = <String, dynamic>{'volumeId': id};
      var changed = false;
      for (final entry in <String, bool>{
        'whitelisted': draft.whitelisted,
        'read': draft.read,
        'write': draft.write,
        'exec': draft.exec,
      }.entries) {
        final fallback = volume[entry.key];
        final resourceDefault = fallback is bool ? fallback : true;
        final hadOverride = stored[id]?.containsKey(entry.key) ?? false;
        if (entry.value != resourceDefault) {
          row[entry.key] = entry.value;
          changed = true;
        } else if (hadOverride) {
          row[entry.key] = null;
          changed = true;
        }
      }
      if (changed) {
        out.add(row);
      }
    }
    return out;
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final patch = _patch();
      await widget.onSave(patch);
      if (!mounted) {
        return;
      }
      setState(() {
        _saving = false;
        _remember(patch);
        _originalPathIDs
          ..clear()
          ..addAll(_paths.map((path) => path.id));
        _removedPathIDs.clear();
      });
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.toString();
        _saving = false;
      });
    }
  }

  void _addPath() {
    setState(() {
      _paths.add(
        _PathDraft(
          id: newSandboxPathID(),
          path: '',
          enabled: true,
          whitelisted: true,
          read: true,
          write: true,
          exec: false,
        ),
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final prefix = widget.keyPrefix;
    final body = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        DropdownButtonFormField<String>(
          key: Key('$prefix-resource'),
          initialValue: _resourceId,
          decoration: const InputDecoration(labelText: 'Resource'),
          items: [
            DropdownMenuItem(value: '', child: Text(widget.emptyChoiceLabel)),
            for (final resource in widget.resources)
              DropdownMenuItem(value: resource.id, child: Text(resource.name)),
          ],
          onChanged: (value) {
            if (value == null) {
              return;
            }
            setState(() {
              _resourceId = value;
              _syncGrants();
            });
          },
        ),
        const SizedBox(height: 12),
        TextField(
          key: Key('$prefix-workspace-root'),
          controller: _workspaceRoot,
          decoration: const InputDecoration(labelText: 'Workspace root'),
        ),
        const SizedBox(height: 16),
        Text('Grant overrides', style: Theme.of(context).textTheme.titleMedium),
        Text(
          'Changes apply to volumes on the selected resource. This cannot add a mount.',
          style: Theme.of(context).textTheme.bodySmall,
        ),
        const SizedBox(height: 8),
        for (final volume in _volumes()) ...[
          _GrantCard(
            keyPrefix: prefix,
            volume: volume,
            draft: _grants[volume['id']],
            onChanged: () => setState(() {}),
          ),
          const SizedBox(height: 8),
        ],
        const SizedBox(height: 8),
        Text('Extra paths', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        for (final path in _paths) ...[
          _PathCard(
            keyPrefix: prefix,
            path: path,
            onChanged: () => setState(() {}),
            onRemove: () {
              setState(() {
                _paths.remove(path);
                if (_originalPathIDs.contains(path.id)) {
                  _removedPathIDs.add(path.id);
                }
                path.dispose();
              });
            },
          ),
          const SizedBox(height: 8),
        ],
        OutlinedButton(
          key: Key('$prefix-path-add'),
          onPressed: _addPath,
          child: const Text('Add extra path'),
        ),
        if (_error != null) ...[
          const SizedBox(height: 12),
          Text(
            _error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ],
        const SizedBox(height: 16),
        Align(
          alignment: Alignment.centerLeft,
          child: FilledButton(
            key: Key(widget.saveKeyName ?? '$prefix-save'),
            onPressed: _saving ? null : _save,
            child: Text(_saving ? 'Saving...' : widget.saveLabel),
          ),
        ),
      ],
    );
    if (widget.embedded) {
      return body;
    }
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: body,
    );
  }
}

class _GrantCard extends StatelessWidget {
  const _GrantCard({
    required this.keyPrefix,
    required this.volume,
    required this.draft,
    required this.onChanged,
  });

  final String keyPrefix;
  final Map<String, dynamic> volume;
  final _GrantDraft? draft;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final flags = draft;
    if (flags == null) {
      return const SizedBox.shrink();
    }
    final id = volume['id'] as String? ?? '';
    final name = volume['name'] as String? ?? id;
    final target = volume['target'] as String? ?? '';
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('$name -> $target'),
            SwitchListTile(
              key: Key('$keyPrefix-grant-$id-whitelisted'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Whitelisted'),
              value: flags.whitelisted,
              onChanged: (value) {
                flags.whitelisted = value;
                onChanged();
              },
            ),
            SwitchListTile(
              key: Key('$keyPrefix-grant-$id-read'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Read'),
              value: flags.read,
              onChanged: flags.whitelisted
                  ? (value) {
                      flags.read = value;
                      onChanged();
                    }
                  : null,
            ),
            SwitchListTile(
              key: Key('$keyPrefix-grant-$id-write'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Write'),
              value: flags.write,
              onChanged: flags.whitelisted
                  ? (value) {
                      flags.write = value;
                      onChanged();
                    }
                  : null,
            ),
            SwitchListTile(
              key: Key('$keyPrefix-grant-$id-exec'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Exec'),
              value: flags.exec,
              onChanged: flags.whitelisted
                  ? (value) {
                      flags.exec = value;
                      onChanged();
                    }
                  : null,
            ),
          ],
        ),
      ),
    );
  }
}

class _PathCard extends StatelessWidget {
  const _PathCard({
    required this.keyPrefix,
    required this.path,
    required this.onChanged,
    required this.onRemove,
  });

  final String keyPrefix;
  final _PathDraft path;
  final VoidCallback onChanged;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          children: [
            TextField(
              key: Key('$keyPrefix-path-${path.id}-path'),
              controller: path.pathController,
              decoration: const InputDecoration(labelText: 'Path'),
            ),
            SwitchListTile(
              key: Key('$keyPrefix-path-${path.id}-enabled'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Enabled'),
              value: path.enabled,
              onChanged: (value) {
                path.enabled = value;
                onChanged();
              },
            ),
            SwitchListTile(
              key: Key('$keyPrefix-path-${path.id}-whitelisted'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Whitelisted'),
              value: path.whitelisted,
              onChanged: (value) {
                path.whitelisted = value;
                onChanged();
              },
            ),
            SwitchListTile(
              key: Key('$keyPrefix-path-${path.id}-read'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Read'),
              value: path.read,
              onChanged: path.whitelisted
                  ? (value) {
                      path.read = value;
                      onChanged();
                    }
                  : null,
            ),
            SwitchListTile(
              key: Key('$keyPrefix-path-${path.id}-write'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Write'),
              value: path.write,
              onChanged: path.whitelisted
                  ? (value) {
                      path.write = value;
                      onChanged();
                    }
                  : null,
            ),
            SwitchListTile(
              key: Key('$keyPrefix-path-${path.id}-exec'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Exec'),
              value: path.exec,
              onChanged: path.whitelisted
                  ? (value) {
                      path.exec = value;
                      onChanged();
                    }
                  : null,
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
