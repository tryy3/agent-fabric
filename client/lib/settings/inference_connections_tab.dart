import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:agent_fabric_client/core/settings_load_state.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';

class InferenceConnectionsTab extends StatefulWidget {
  const InferenceConnectionsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<InferenceConnectionsTab> createState() =>
      _InferenceConnectionsTabState();
}

class _InferenceConnectionsTabState extends State<InferenceConnectionsTab> {
  SettingsLoadState<InferenceConnection> _state = const SettingsLoading();

  @override
  void initState() {
    super.initState();
    _startReload();
  }

  void _startReload() {
    unawaited(
      _reload().catchError((Object e, StackTrace s) {
        AppLog.record('connections reload: $e', s);
      }),
    );
  }

  void _onAddInferenceConnection() {
    unawaited(
      _openEditor().catchError((Object e, StackTrace s) {
        AppLog.record('connections open editor: $e', s);
      }),
    );
  }

  void _onEditInferenceConnection(InferenceConnection provider) {
    unawaited(
      _openEditor(provider: provider).catchError((Object e, StackTrace s) {
        AppLog.record('connections edit: $e', s);
      }),
    );
  }

  void _onDeleteInferenceConnection(InferenceConnection provider) {
    unawaited(
      _confirmDelete(provider).catchError((Object e, StackTrace s) {
        AppLog.record('connections delete: $e', s);
      }),
    );
  }

  void _onRefreshModels(String id) {
    unawaited(
      _refreshModels(id).catchError((Object e, StackTrace s) {
        AppLog.record('connections refresh models: $e', s);
      }),
    );
  }

  Future<void> _reload() async {
    setState(() => _state = const SettingsLoading());
    try {
      final list = await widget.catalog.listInferenceConnections();
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsReady(list));
    } on Object catch (e, s) {
      AppLog.record('connections reload failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsFailed(operatorFailureFrom(e)));
    }
  }

  Future<void> _openEditor({InferenceConnection? provider}) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) => _CreateInferenceConnectionDialog(
        catalog: widget.catalog,
        provider: provider,
      ),
    );
    if (saved == true) {
      await _reload();
    }
  }

  Future<void> _confirmDelete(InferenceConnection provider) async {
    var using = <Assistant>[];
    var agentsLoadFailed = false;
    try {
      final agents = await widget.catalog.listAssistants();
      using = agents
          .where((a) => a.inferenceConnectionId == provider.id)
          .toList();
    } on Object catch (e, s) {
      AppLog.record('connections list assistants for delete: $e', s);
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
            body = 'Could not load assistants. Delete ${provider.name} anyway?';
          } else if (using.isEmpty) {
            body = 'Delete ${provider.name}?';
          } else {
            body =
                'Deleting ${provider.name} will unset their connection and '
                'model for: ${using.map((a) => a.name).join(', ')}';
          }
          return AlertDialog(
            title: const Text('Delete connection?'),
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
      await widget.catalog.deleteInferenceConnection(provider.id);
      await _reload();
    } on Object catch (e, s) {
      AppLog.record('connections delete failed: $e', s);
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
      if (current is! SettingsReady<InferenceConnection>) {
        return;
      }
      setState(() {
        _state = SettingsReady([
          for (final provider in current.items)
            if (provider.id == id) updated else provider,
        ]);
      });
    } on Object catch (e, s) {
      AppLog.record('connections refresh models failed: $e', s);
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
      body: SettingsLoadBody<InferenceConnection>(
        state: _state,
        emptyLabel: 'No connections',
        onRetry: _startReload,
        itemBuilder: (context, provider) {
          final updated = provider.modelsUpdatedAt == null
              ? 'never'
              : provider.modelsUpdatedAt!.toUtc().toIso8601String();
          return Semantics(
            button: true,
            label: 'Connection ${provider.name}',
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
              onTap: () => _onEditInferenceConnection(provider),
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
                    label: 'Delete connection ${provider.name}',
                    child: IconButton(
                      key: Key('delete-provider-${provider.id}'),
                      tooltip: 'Delete connection',
                      icon: const Icon(Icons.delete),
                      onPressed: () => _onDeleteInferenceConnection(provider),
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
        label: 'Add connection',
        child: FloatingActionButton(
          onPressed: _onAddInferenceConnection,
          tooltip: 'Add connection',
          child: const Icon(Icons.add),
        ),
      ),
    );
  }
}

class _CreateInferenceConnectionDialog extends StatefulWidget {
  const _CreateInferenceConnectionDialog({
    required this.catalog,
    this.provider,
  });

  final CatalogClient catalog;
  final InferenceConnection? provider;

  @override
  State<_CreateInferenceConnectionDialog> createState() =>
      _CreateInferenceConnectionDialogState();
}

class _CreateInferenceConnectionDialogState
    extends State<_CreateInferenceConnectionDialog> {
  late final TextEditingController _name;
  late final TextEditingController _baseUrl;
  late final TextEditingController _apiKey;
  late String _type;
  String? _error;
  bool _saving = false;

  bool get _editing => widget.provider != null;

  bool get _hasFixedBaseUrl => hasFixedBaseUrl(_type);

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
      if ((hasFixedBaseUrl(value) || value == providerTypeUnslothStudio) &&
          _name.text.trim().isEmpty) {
        _name.text = providerTypeLabel(value);
      }
    });
  }

  void _onSubmit() {
    unawaited(
      _submit().catchError((Object e, StackTrace s) {
        AppLog.record('connection submit: $e', s);
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
        await widget.catalog.createInferenceConnection(
          name: _name.text,
          type: _type,
          baseUrl: _hasFixedBaseUrl ? '' : _baseUrl.text,
          apiKey: _apiKey.text,
        );
      } else {
        await widget.catalog.updateInferenceConnection(
          provider.id,
          name: _name.text,
          baseUrl: hasFixedBaseUrl(provider.type) ? null : _baseUrl.text,
          apiKey: _apiKey.text,
        );
      }
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(true);
    } on Object catch (e, s) {
      AppLog.record('connection submit failed: $e', s);
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
      title: Text(_editing ? 'Edit connection' : 'Add connection'),
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
                  value: providerTypeUnslothStudio,
                  child: Text('Unsloth Studio'),
                ),
                DropdownMenuItem(
                  value: providerTypeBergetAI,
                  child: Text('Berget AI'),
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
          if (!_hasFixedBaseUrl)
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
