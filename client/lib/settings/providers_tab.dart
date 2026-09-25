import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:agent_fabric_client/core/settings_load_state.dart';
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
  SettingsLoadState<Provider> _state = const SettingsLoading();

  @override
  void initState() {
    super.initState();
    _startReload();
  }

  void _startReload() {
    unawaited(
      _reload().catchError((Object e, StackTrace s) {
        AppLog.record('providers reload: $e', s);
      }),
    );
  }

  void _onAddProvider() {
    unawaited(
      _openEditor().catchError((Object e, StackTrace s) {
        AppLog.record('providers open editor: $e', s);
      }),
    );
  }

  void _onEditProvider(Provider provider) {
    unawaited(
      _openEditor(provider: provider).catchError((Object e, StackTrace s) {
        AppLog.record('providers edit: $e', s);
      }),
    );
  }

  void _onDeleteProvider(Provider provider) {
    unawaited(
      _confirmDelete(provider).catchError((Object e, StackTrace s) {
        AppLog.record('providers delete: $e', s);
      }),
    );
  }

  void _onRefreshModels(String id) {
    unawaited(
      _refreshModels(id).catchError((Object e, StackTrace s) {
        AppLog.record('providers refresh models: $e', s);
      }),
    );
  }

  Future<void> _reload() async {
    setState(() => _state = const SettingsLoading());
    try {
      final list = await widget.catalog.listProviders();
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsReady(list));
    } on Object catch (e, s) {
      AppLog.record('providers reload failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsFailed(operatorFailureFrom(e)));
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
      using = agents.where((agent) => agent.providerId == provider.id).toList();
    } on Object catch (e, s) {
      AppLog.record('providers list agents for delete: $e', s);
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
            body = 'Could not load agents. Delete ${provider.name} anyway?';
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
    } on Object catch (e, s) {
      AppLog.record('providers delete failed: $e', s);
      if (!mounted) {
        return;
      }
      _showActionError(e);
    }
  }

  Future<void> _refreshModels(String id) async {
    try {
      final updated = await widget.catalog.refreshModels(id);
      if (!mounted) {
        return;
      }
      final current = _state;
      if (current is! SettingsReady<Provider>) {
        return;
      }
      setState(() {
        _state = SettingsReady([
          for (final provider in current.items)
            if (provider.id == id) updated else provider,
        ]);
      });
    } on Object catch (e, s) {
      AppLog.record('providers refresh models failed: $e', s);
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
      body: SettingsLoadBody<Provider>(
        state: _state,
        emptyLabel: 'No providers',
        onRetry: _startReload,
        itemBuilder: (context, provider) {
          final updated = provider.modelsUpdatedAt == null
              ? 'never'
              : provider.modelsUpdatedAt!.toUtc().toIso8601String();
          return Semantics(
            button: true,
            label: 'Provider ${provider.name}',
            child: ListTile(
              title: Text(provider.name),
              subtitle: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(provider.typeLabel),
                  if (provider.models.isEmpty)
                    const Text('No cached models')
                  else
                    ...provider.models.map((m) => Text(m.name)),
                  Text('Last updated: $updated'),
                ],
              ),
              isThreeLine: true,
              onTap: () => _onEditProvider(provider),
              trailing: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Semantics(
                    button: true,
                    label: 'Refresh models for ${provider.name}',
                    child: TextButton(
                      onPressed: () => _onRefreshModels(provider.id),
                      child: const Text('Refresh models'),
                    ),
                  ),
                  Semantics(
                    button: true,
                    label: 'Delete provider ${provider.name}',
                    child: IconButton(
                      key: Key('delete-provider-${provider.id}'),
                      tooltip: 'Delete provider',
                      icon: const Icon(Icons.delete),
                      onPressed: () => _onDeleteProvider(provider),
                    ),
                  ),
                ],
              ),
            ),
          );
        },
      ),
      floatingActionButton: Semantics(
        button: true,
        label: 'Add provider',
        child: FloatingActionButton(
          onPressed: _onAddProvider,
          tooltip: 'Add provider',
          child: const Icon(Icons.add),
        ),
      ),
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
  late String _type;
  String? _error;
  bool _saving = false;

  bool get _editing => widget.provider != null;

  bool get _isOpenCode => isOpenCodeProviderType(_type);

  @override
  void initState() {
    super.initState();
    final provider = widget.provider;
    _type = provider?.type ?? providerTypeOpenAICompatible;
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

  void _onTypeChanged(String? value) {
    if (value == null || _editing) {
      return;
    }
    setState(() {
      _type = value;
      if (isOpenCodeProviderType(value) && _name.text.trim().isEmpty) {
        _name.text = providerTypeLabel(value);
      }
    });
  }

  void _onSubmit() {
    unawaited(
      _submit().catchError((Object e, StackTrace s) {
        AppLog.record('provider submit: $e', s);
      }),
    );
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
          type: _type,
          baseUrl: _isOpenCode ? '' : _baseUrl.text,
          apiKey: _apiKey.text,
        );
      } else {
        await widget.catalog.updateProvider(
          provider.id,
          name: _name.text,
          baseUrl: provider.isOpenCode ? null : _baseUrl.text,
          apiKey: _apiKey.text,
        );
      }
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(true);
    } on Object catch (e, s) {
      AppLog.record('provider submit failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _error = operatorMessageFromError(e);
        _saving = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(_editing ? 'Edit provider' : 'Add provider'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (!_editing)
            DropdownButtonFormField<String>(
              key: const Key('provider-type'),
              initialValue: _type,
              decoration: const InputDecoration(labelText: 'Type'),
              items: const [
                DropdownMenuItem(
                  value: providerTypeOpenAICompatible,
                  child: Text('Custom'),
                ),
                DropdownMenuItem(
                  value: providerTypeOpenCodeZen,
                  child: Text('OpenCode Zen'),
                ),
                DropdownMenuItem(
                  value: providerTypeOpenCodeGo,
                  child: Text('OpenCode Go'),
                ),
              ],
              onChanged: _onTypeChanged,
            )
          else
            InputDecorator(
              decoration: const InputDecoration(labelText: 'Type'),
              child: Text(providerTypeLabel(_type)),
            ),
          TextField(
            controller: _name,
            decoration: const InputDecoration(labelText: 'Name'),
          ),
          if (!_isOpenCode)
            TextField(
              key: const Key('provider-base-url'),
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
          onPressed: _saving ? null : _onSubmit,
          child: Text(_editing ? 'Save' : 'Create'),
        ),
      ],
    );
  }
}
