import 'dart:math';

import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';

String newSandboxVolumeID() => _newPrefixedID('vol_');

String newSandboxPathID() => _newPrefixedID('path_');

String _newPrefixedID(String prefix) {
  final rng = Random.secure();
  final bytes = List<int>.generate(8, (_) => rng.nextInt(256));
  final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  return '$prefix$hex';
}

class ResourcesTab extends StatefulWidget {
  const ResourcesTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<ResourcesTab> createState() => _ResourcesTabState();
}

class _ResourcesTabState extends State<ResourcesTab> {
  List<Resource> _resources = [];
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  Future<void> _reload() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final resources = await widget.catalog.listResources();
      if (!mounted) {
        return;
      }
      setState(() {
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

  Future<void> _openEditor({Resource? resource}) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) =>
          _ResourceEditorDialog(catalog: widget.catalog, resource: resource),
    );
    if (saved == true) {
      await _reload();
    }
  }

  Future<void> _confirmDelete(Resource resource) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) {
        return AlertDialog(
          title: const Text('Delete resource?'),
          content: Text(
            'Delete ${resource.name}? The Docker container and volumes are left in place.',
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
    if (confirmed != true || !mounted) {
      return;
    }
    try {
      await widget.catalog.deleteResource(resource.id);
      await _reload();
    } catch (e) {
      if (!mounted) {
        return;
      }
      final message = e.toString();
      setState(() {
        _error = message.contains('resource in use')
            ? 'resource in use'
            : message;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: _buildBody(),
      floatingActionButton: FloatingActionButton(
        onPressed: () => _openEditor(),
        tooltip: 'Add resource',
        child: const Icon(Icons.add),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    final list = _resources.isEmpty
        ? const Center(child: Text('No resources'))
        : ListView.builder(
            itemCount: _resources.length,
            itemBuilder: (context, index) {
              final resource = _resources[index];
              return ListTile(
                key: Key('resource-${resource.id}'),
                title: Text(resource.name),
                subtitle: Text(resource.kind),
                trailing: IconButton(
                  key: Key('delete-resource-${resource.id}'),
                  tooltip: 'Delete resource',
                  icon: const Icon(Icons.delete),
                  onPressed: () => _confirmDelete(resource),
                ),
                onTap: () => _openEditor(resource: resource),
              );
            },
          );
    if (_error == null) {
      return list;
    }
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: Align(alignment: Alignment.centerLeft, child: Text(_error!)),
        ),
        Expanded(child: list),
      ],
    );
  }
}

class _VolumeDraft {
  _VolumeDraft({
    required this.id,
    required String name,
    required String target,
    required this.enabled,
    required this.whitelisted,
    required this.read,
    required this.write,
    required this.exec,
  }) : nameController = TextEditingController(text: name),
       targetController = TextEditingController(text: target);

  final String id;
  final TextEditingController nameController;
  final TextEditingController targetController;
  bool enabled;
  bool whitelisted;
  bool read;
  bool write;
  bool exec;

  void dispose() {
    nameController.dispose();
    targetController.dispose();
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': nameController.text.trim(),
    'target': targetController.text.trim(),
    'enabled': enabled,
    'whitelisted': whitelisted,
    'read': read,
    'write': write,
    'exec': exec,
  };
}

class _ResourceEditorDialog extends StatefulWidget {
  const _ResourceEditorDialog({required this.catalog, this.resource});

  final CatalogClient catalog;
  final Resource? resource;

  @override
  State<_ResourceEditorDialog> createState() => _ResourceEditorDialogState();
}

class _ResourceEditorDialogState extends State<_ResourceEditorDialog> {
  late final TextEditingController _name;
  late final TextEditingController _image;
  late final TextEditingController _containerName;
  late final TextEditingController _idleTTL;
  final List<_VolumeDraft> _volumes = [];
  String? _error;
  bool _saving = false;

  bool get _isCreate => widget.resource == null;

  @override
  void initState() {
    super.initState();
    final spec = widget.resource?.spec ?? const <String, dynamic>{};
    _name = TextEditingController(text: widget.resource?.name ?? '');
    _image = TextEditingController(text: spec['image'] as String? ?? '');
    _containerName = TextEditingController(
      text: spec['containerName'] as String? ?? '',
    );
    final ttl = spec['idleTTLSeconds'];
    _idleTTL = TextEditingController(
      text: ttl == null ? (_isCreate ? '3600' : '') : '$ttl',
    );
    final rawVolumes = spec['volumes'];
    if (rawVolumes is List) {
      for (final item in rawVolumes) {
        if (item is! Map) {
          continue;
        }
        final map = Map<String, dynamic>.from(item);
        final id = map['id'] as String? ?? '';
        if (id.isEmpty) {
          continue;
        }
        _volumes.add(
          _VolumeDraft(
            id: id,
            name: map['name'] as String? ?? '',
            target: map['target'] as String? ?? '',
            enabled: map['enabled'] as bool? ?? true,
            whitelisted: map['whitelisted'] as bool? ?? true,
            read: map['read'] as bool? ?? true,
            write: map['write'] as bool? ?? true,
            exec: map['exec'] as bool? ?? true,
          ),
        );
      }
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _image.dispose();
    _containerName.dispose();
    _idleTTL.dispose();
    for (final volume in _volumes) {
      volume.dispose();
    }
    super.dispose();
  }

  Map<String, dynamic> _spec() {
    final ttl = int.tryParse(_idleTTL.text.trim());
    return {
      'image': _image.text.trim(),
      'containerName': _containerName.text.trim(),
      if (ttl != null) 'idleTTLSeconds': ttl,
      'volumes': [for (final volume in _volumes) volume.toJson()],
    };
  }

  Future<void> _save() async {
    final ttlText = _idleTTL.text.trim();
    if (ttlText.isNotEmpty && int.tryParse(ttlText) == null) {
      setState(() {
        _error = 'Idle timeout must be a number of seconds';
      });
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final name = _name.text.trim();
      final spec = _spec();
      if (_isCreate) {
        await widget.catalog.createResource(
          name: name,
          kind: 'container',
          spec: spec,
        );
      } else {
        await widget.catalog.updateResource(
          widget.resource!.id,
          name: name,
          spec: spec,
        );
      }
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(true);
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

  void _addVolume() {
    setState(() {
      _volumes.add(
        _VolumeDraft(
          id: newSandboxVolumeID(),
          name: '',
          target: '',
          enabled: true,
          whitelisted: true,
          read: true,
          write: true,
          exec: true,
        ),
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(_isCreate ? 'Add resource' : 'Edit resource'),
      content: SizedBox(
        width: 560,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              TextField(
                key: const Key('resource-name'),
                controller: _name,
                decoration: const InputDecoration(labelText: 'Name'),
              ),
              TextField(
                key: const Key('resource-image'),
                controller: _image,
                decoration: const InputDecoration(labelText: 'Image'),
              ),
              TextField(
                key: const Key('resource-container-name'),
                controller: _containerName,
                decoration: const InputDecoration(labelText: 'Container name'),
              ),
              TextField(
                key: const Key('resource-idle-ttl'),
                controller: _idleTTL,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(
                  labelText: 'Idle timeout (seconds)',
                ),
              ),
              const SizedBox(height: 16),
              Text('Volumes', style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 8),
              for (final volume in _volumes) ...[
                _VolumeCard(
                  volume: volume,
                  onChanged: () => setState(() {}),
                  onRemove: () {
                    setState(() {
                      _volumes.remove(volume);
                      volume.dispose();
                    });
                  },
                ),
                const SizedBox(height: 8),
              ],
              OutlinedButton(
                key: const Key('resource-volume-add'),
                onPressed: _addVolume,
                child: const Text('Add volume'),
              ),
              if (_error != null) ...[
                const SizedBox(height: 12),
                Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _saving ? null : () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        TextButton(
          key: const Key('resource-save'),
          onPressed: _saving ? null : _save,
          child: const Text('Save'),
        ),
      ],
    );
  }
}

class _VolumeCard extends StatelessWidget {
  const _VolumeCard({
    required this.volume,
    required this.onChanged,
    required this.onRemove,
  });

  final _VolumeDraft volume;
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
              key: Key('resource-volume-${volume.id}-name'),
              controller: volume.nameController,
              decoration: const InputDecoration(labelText: 'Volume name'),
            ),
            TextField(
              key: Key('resource-volume-${volume.id}-target'),
              controller: volume.targetController,
              decoration: const InputDecoration(labelText: 'Target'),
            ),
            SwitchListTile(
              key: Key('resource-volume-${volume.id}-enabled'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Enabled'),
              value: volume.enabled,
              onChanged: (value) {
                volume.enabled = value;
                onChanged();
              },
            ),
            SwitchListTile(
              key: Key('resource-volume-${volume.id}-whitelisted'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Whitelisted'),
              value: volume.whitelisted,
              onChanged: (value) {
                volume.whitelisted = value;
                onChanged();
              },
            ),
            SwitchListTile(
              key: Key('resource-volume-${volume.id}-read'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Read'),
              value: volume.read,
              onChanged: volume.whitelisted
                  ? (value) {
                      volume.read = value;
                      onChanged();
                    }
                  : null,
            ),
            SwitchListTile(
              key: Key('resource-volume-${volume.id}-write'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Write'),
              value: volume.write,
              onChanged: volume.whitelisted
                  ? (value) {
                      volume.write = value;
                      onChanged();
                    }
                  : null,
            ),
            SwitchListTile(
              key: Key('resource-volume-${volume.id}-exec'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Exec'),
              value: volume.exec,
              onChanged: volume.whitelisted
                  ? (value) {
                      volume.exec = value;
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
