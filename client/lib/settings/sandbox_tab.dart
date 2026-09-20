import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';

class SandboxTab extends StatefulWidget {
  const SandboxTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<SandboxTab> createState() => _SandboxTabState();
}

class _SandboxTabState extends State<SandboxTab> {
  final _workspaceRoot = TextEditingController();
  final _image = TextEditingController();
  final _idleTTL = TextEditingController();
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
    super.dispose();
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
        final ttl = sandbox['idleTTLSeconds'];
        _idleTTL.text = ttl == null ? '' : '$ttl';
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
          if (ttl != null) 'idleTTLSeconds': ttl,
        },
      );
      if (!mounted) {
        return;
      }
      setState(() {
        _saving = false;
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

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(
          'Global sandbox defaults',
          style: Theme.of(context).textTheme.titleMedium,
        ),
        const SizedBox(height: 4),
        Text(
          'These apply to the next prompt. Project and agent overlays can still override them.',
          style: Theme.of(context).textTheme.bodySmall
              ?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant),
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
          key: const Key('sandbox-idle-ttl'),
          controller: _idleTTL,
          keyboardType: TextInputType.number,
          decoration: const InputDecoration(labelText: 'Idle TTL (seconds)'),
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
    );
  }
}
