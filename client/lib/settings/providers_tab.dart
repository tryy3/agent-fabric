import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';

class ProvidersTab extends StatefulWidget {
  const ProvidersTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<ProvidersTab> createState() => _ProvidersTabState();
}

class _ProvidersTabState extends State<ProvidersTab> {
  List<Provider> _providers = [];
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
      final list = await widget.catalog.listProviders();
      if (!mounted) {
        return;
      }
      setState(() {
        _providers = list;
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

  Future<void> _openEditor({Provider? provider}) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) =>
          _CreateProviderDialog(catalog: widget.catalog, provider: provider),
    );
    if (saved == true) {
      await _reload();
    }
  }

  Future<void> _confirmDelete(Provider provider) async {
    var using = <Agent>[];
    var agentsLoadFailed = false;
    try {
      final agents = await widget.catalog.listAgents();
      using = agents
          .where((agent) => agent.providerId == provider.id)
          .toList();
    } catch (_) {
      agentsLoadFailed = true;
    }
    if (!mounted) {
      return;
    }
    try {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) {
          final String body;
          if (agentsLoadFailed) {
            body =
                'Could not load agents. Delete ${provider.name} anyway?';
          } else if (using.isEmpty) {
            body = 'Delete ${provider.name}?';
          } else {
            body =
                'Deleting ${provider.name} will unset their provider and '
                'model for: ${using.map((a) => a.name).join(', ')}';
          }
          return AlertDialog(
            title: const Text('Delete provider?'),
            content: Text(body),
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
      await widget.catalog.deleteProvider(provider.id);
      await _reload();
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.toString();
      });
    }
  }

  Future<void> _refreshModels(String id) async {
    try {
      final updated = await widget.catalog.refreshModels(id);
      if (!mounted) {
        return;
      }
      setState(() {
        _providers = [
          for (final provider in _providers)
            if (provider.id == id) updated else provider,
        ];
      });
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.toString();
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: _buildBody(),
      floatingActionButton: FloatingActionButton(
        onPressed: _openEditor,
        tooltip: 'Add provider',
        child: const Icon(Icons.add),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    final list = _providers.isEmpty
        ? const Center(child: Text('No providers'))
        : ListView.builder(
            itemCount: _providers.length,
            itemBuilder: (context, index) {
              final provider = _providers[index];
              final updated = provider.modelsUpdatedAt == null
                  ? 'never'
                  : provider.modelsUpdatedAt!.toUtc().toIso8601String();
              return ListTile(
                title: Text(provider.name),
                subtitle: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (provider.models.isEmpty)
                      const Text('No cached models')
                    else
                      ...provider.models.map((m) => Text(m.name)),
                    Text('Last updated: $updated'),
                  ],
                ),
                isThreeLine: true,
                onTap: () => _openEditor(provider: provider),
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    TextButton(
                      onPressed: () => _refreshModels(provider.id),
                      child: const Text('Refresh models'),
                    ),
                    IconButton(
                      key: Key('delete-provider-${provider.id}'),
                      tooltip: 'Delete provider',
                      icon: const Icon(Icons.delete),
                      onPressed: () => _confirmDelete(provider),
                    ),
                  ],
                ),
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

class _CreateProviderDialog extends StatefulWidget {
  const _CreateProviderDialog({required this.catalog, this.provider});

  final CatalogClient catalog;
  final Provider? provider;

  @override
  State<_CreateProviderDialog> createState() => _CreateProviderDialogState();
}

class _CreateProviderDialogState extends State<_CreateProviderDialog> {
  late final TextEditingController _name;
  late final TextEditingController _baseUrl;
  late final TextEditingController _apiKey;
  String? _error;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final provider = widget.provider;
    _name = TextEditingController(text: provider?.name ?? '');
    _baseUrl = TextEditingController(text: provider?.baseUrl ?? '');
    _apiKey = TextEditingController(text: provider?.apiKey ?? '');
  }

  @override
  void dispose() {
    _name.dispose();
    _baseUrl.dispose();
    _apiKey.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final provider = widget.provider;
      if (provider == null) {
        await widget.catalog.createProvider(
          name: _name.text,
          type: 'openai_compatible',
          baseUrl: _baseUrl.text,
          apiKey: _apiKey.text,
        );
      } else {
        await widget.catalog.updateProvider(
          provider.id,
          name: _name.text,
          baseUrl: _baseUrl.text,
          apiKey: _apiKey.text,
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

  @override
  Widget build(BuildContext context) {
    final editing = widget.provider != null;
    return AlertDialog(
      title: Text(editing ? 'Edit provider' : 'Add provider'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          TextField(
            controller: _name,
            decoration: const InputDecoration(labelText: 'Name'),
          ),
          TextField(
            controller: _baseUrl,
            decoration: const InputDecoration(labelText: 'Base URL'),
          ),
          TextField(
            controller: _apiKey,
            obscureText: true,
            decoration: const InputDecoration(labelText: 'API key'),
          ),
          if (_error != null) Text(_error!),
        ],
      ),
      actions: [
        TextButton(
          onPressed: _saving ? null : () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: _saving ? null : _submit,
          child: Text(editing ? 'Save' : 'Create'),
        ),
      ],
    );
  }
}
