import 'dart:math';

import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';

String newSandboxVolumeID() {
  final rng = Random.secure();
  final bytes = List<int>.generate(8, (_) => rng.nextInt(256));
  final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  return 'vol_$hex';
}

class SandboxTab extends StatefulWidget {
  const SandboxTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<SandboxTab> createState() => _SandboxTabState();
}

class _VolumeDraft {
  _VolumeDraft({
    required this.id,
    required String name,
    required String target,
    required this.enabled,
    required this.write,
  }) : nameController = TextEditingController(text: name),
       targetController = TextEditingController(text: target);

  final String id;
  final TextEditingController nameController;
  final TextEditingController targetController;
  bool enabled;
  bool write;

  void dispose() {
    nameController.dispose();
    targetController.dispose();
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': nameController.text.trim(),
    'target': targetController.text.trim(),
    'enabled': enabled,
    'write': write,
  };
}

class _SandboxTabState extends State<SandboxTab> {
  final _workspaceRoot = TextEditingController();
  final _image = TextEditingController();
  final _idleTTL = TextEditingController();
  final _containerName = TextEditingController();
  final _originalVolumeIDs = <String>{};
  final _removedVolumeIDs = <String>{};
  final List<_VolumeDraft> _volumes = [];
  String _kind = 'docker';
  String? _error;
  bool _loading = true;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  @override
  void dispose() {
    _workspaceRoot.dispose();
    _image.dispose();
    _idleTTL.dispose();
    _containerName.dispose();
    for (final volume in _volumes) {
      volume.dispose();
    }
    super.dispose();
  }

  void _replaceVolumes(List<_VolumeDraft> next) {
    for (final volume in _volumes) {
      volume.dispose();
    }
    _volumes
      ..clear()
      ..addAll(next);
    _removedVolumeIDs.clear();
    _originalVolumeIDs
      ..clear()
      ..addAll(next.map((volume) => volume.id));
  }

  List<_VolumeDraft> _volumesFrom(dynamic raw) {
    if (raw is! List) {
      return <_VolumeDraft>[];
    }
    final out = <_VolumeDraft>[];
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
        _VolumeDraft(
          id: id,
          name: map['name'] as String? ?? '',
          target: map['target'] as String? ?? '',
          enabled: map['enabled'] as bool? ?? true,
          write: map['write'] as bool? ?? true,
        ),
      );
    }
    return out;
  }

  Future<void> _reload() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final settings = await widget.catalog.getSettings();
      if (!mounted) {
        return;
      }
      final sandbox = settings.sandbox;
      setState(() {
        _kind = sandbox['kind'] as String? ?? 'docker';
        _workspaceRoot.text = sandbox['workspaceRoot'] as String? ?? '';
        _image.text = sandbox['image'] as String? ?? '';
        _containerName.text = sandbox['containerName'] as String? ?? '';
        final ttl = sandbox['idleTTLSeconds'];
        _idleTTL.text = ttl == null ? '' : '$ttl';
        _replaceVolumes(_volumesFrom(sandbox['volumes']));
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

  Future<void> _save() async {
    final ttl = int.tryParse(_idleTTL.text.trim());
    if (_idleTTL.text.trim().isNotEmpty && ttl == null) {
      setState(() {
        _error = 'Idle TTL must be a number of seconds';
      });
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.catalog.patchSettings(
        sandbox: {
          'kind': _kind,
          'workspaceRoot': _workspaceRoot.text.trim(),
          'image': _image.text.trim(),
          'containerName': _containerName.text.trim(),
          'idleTTLSeconds': ?ttl,
          'volumes': [
            for (final volume in _volumes) volume.toJson(),
            for (final id in _removedVolumeIDs) {'id': id, 'enabled': false},
          ],
        },
      );
      if (!mounted) {
        return;
      }
      setState(() {
        _saving = false;
        _originalVolumeIDs
          ..clear()
          ..addAll(_volumes.map((volume) => volume.id));
        _removedVolumeIDs.clear();
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

  void _addVolume() {
    setState(() {
      _volumes.add(
        _VolumeDraft(
          id: newSandboxVolumeID(),
          name: '',
          target: '',
          enabled: true,
          write: true,
        ),
      );
    });
  }

  void _removeVolume(_VolumeDraft volume) {
    setState(() {
      _volumes.remove(volume);
      volume.dispose();
      if (_originalVolumeIDs.contains(volume.id)) {
        _removedVolumeIDs.add(volume.id);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    return Scaffold(
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Global sandbox defaults',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 4),
            Text(
              'These apply to the next prompt. Project and agent overlays can still override them.',
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ],
            const SizedBox(height: 16),
            DropdownButtonFormField<String>(
              key: const Key('sandbox-kind'),
              decoration: const InputDecoration(labelText: 'Kind'),
              initialValue: _kind,
              items: const [
                DropdownMenuItem(value: 'docker', child: Text('docker')),
                DropdownMenuItem(value: 'local', child: Text('local')),
              ],
              onChanged: (value) {
                if (value != null) {
                  setState(() {
                    _kind = value;
                  });
                }
              },
            ),
            const SizedBox(height: 16),
            TextField(
              key: const Key('sandbox-workspace-root'),
              controller: _workspaceRoot,
              decoration: const InputDecoration(labelText: 'Workspace root'),
            ),
            const SizedBox(height: 16),
            TextField(
              key: const Key('sandbox-image'),
              controller: _image,
              decoration: const InputDecoration(labelText: 'Image'),
            ),
            const SizedBox(height: 16),
            TextField(
              key: const Key('sandbox-container-name'),
              controller: _containerName,
              decoration: const InputDecoration(
                labelText: 'Container name template',
              ),
            ),
            const SizedBox(height: 16),
            TextField(
              key: const Key('sandbox-idle-ttl'),
              controller: _idleTTL,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                labelText: 'Idle TTL (seconds)',
              ),
            ),
            const SizedBox(height: 24),
            Text('Volumes', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 4),
            Text(
              'Named Docker volumes. Projects that resolve the same name share files.',
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 12),
            for (final volume in _volumes) ...[
              _VolumeCard(
                volume: volume,
                onChanged: () => setState(() {}),
                onRemove: () => _removeVolume(volume),
              ),
              const SizedBox(height: 12),
            ],
            Align(
              alignment: Alignment.centerLeft,
              child: OutlinedButton(
                key: const Key('sandbox-volume-add'),
                onPressed: _addVolume,
                child: const Text('Add volume'),
              ),
            ),
            const SizedBox(height: 24),
            Align(
              alignment: Alignment.centerLeft,
              child: FilledButton(
                key: const Key('sandbox-save'),
                onPressed: _saving ? null : _save,
                child: Text(_saving ? 'Saving…' : 'Save'),
              ),
            ),
          ],
        ),
      ),
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
      key: Key('sandbox-volume-${volume.id}'),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          children: [
            TextField(
              key: Key('sandbox-volume-${volume.id}-name'),
              controller: volume.nameController,
              decoration: const InputDecoration(labelText: 'Name template'),
            ),
            const SizedBox(height: 12),
            TextField(
              key: Key('sandbox-volume-${volume.id}-target'),
              controller: volume.targetController,
              decoration: const InputDecoration(labelText: 'Target'),
            ),
            SwitchListTile(
              key: Key('sandbox-volume-${volume.id}-enabled'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Enabled'),
              value: volume.enabled,
              onChanged: (value) {
                volume.enabled = value;
                onChanged();
              },
            ),
            SwitchListTile(
              key: Key('sandbox-volume-${volume.id}-write'),
              contentPadding: EdgeInsets.zero,
              title: const Text('Write'),
              value: volume.write,
              onChanged: (value) {
                volume.write = value;
                onChanged();
              },
            ),
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton(
                key: Key('sandbox-volume-${volume.id}-remove'),
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
