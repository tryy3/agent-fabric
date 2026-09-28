import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:agent_fabric_client/core/settings_load_state.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';

class AgentsTab extends StatefulWidget {
  const AgentsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<AgentsTab> createState() => _AgentsTabState();
}

class _AgentsTabState extends State<AgentsTab> {
  SettingsLoadState<Agent> _state = const SettingsLoading();
  List<Provider> _providers = const [];

  @override
  void initState() {
    super.initState();
    _startReload();
  }

  void _startReload() {
    unawaited(
      _reload().catchError((Object e, StackTrace s) {
        AppLog.record('agents reload: $e', s);
      }),
    );
  }

  void _onAddAgent() {
    unawaited(
      _openEditor().catchError((Object e, StackTrace s) {
        AppLog.record('agents open editor: $e', s);
      }),
    );
  }

  void _onEditAgent(Agent agent) {
    unawaited(
      _openEditor(agent: agent).catchError((Object e, StackTrace s) {
        AppLog.record('agents edit: $e', s);
      }),
    );
  }

  void _onDeleteAgent(Agent agent) {
    unawaited(
      _confirmDelete(agent).catchError((Object e, StackTrace s) {
        AppLog.record('agents delete: $e', s);
      }),
    );
  }

  Future<void> _reload() async {
    setState(() => _state = const SettingsLoading());
    try {
      final agents = await widget.catalog.listAgents();
      final providers = await widget.catalog.listProviders();
      if (!mounted) {
        return;
      }
      setState(() {
        _providers = providers;
        _state = SettingsReady(agents);
      });
    } on Object catch (e, s) {
      AppLog.record('agents reload failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsFailed(operatorFailureFrom(e)));
    }
  }

  Future<void> _openEditor({Agent? agent}) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) => _AgentEditorDialog(
        catalog: widget.catalog,
        providers: _providers,
        agent: agent,
      ),
    );
    if (saved == true) {
      await _reload();
    }
  }

  Future<void> _confirmDelete(Agent agent) async {
    try {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) {
          return AlertDialog(
            title: const Text('Delete agent?'),
            content: Text('Delete ${agent.name}?'),
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
      await widget.catalog.deleteAgent(agent.id);
      await _reload();
    } on Object catch (e, s) {
      AppLog.record('agents delete failed: $e', s);
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
      body: SettingsLoadBody<Agent>(
        state: _state,
        emptyLabel: 'No agents',
        onRetry: _startReload,
        itemBuilder: (context, agent) {
          return Semantics(
            button: true,
            label: 'Agent ${agent.name}',
            child: ListTile(
              title: Text(agent.name),
              subtitle: Text(
                !agent.isComplete
                    ? 'Needs provider'
                    : (agent.description.isEmpty
                          ? (agent.defaultModel ?? '')
                          : agent.description),
              ),
              trailing: Semantics(
                button: true,
                label: 'Delete agent ${agent.name}',
                child: IconButton(
                  key: Key('delete-agent-${agent.id}'),
                  tooltip: 'Delete agent',
                  icon: const Icon(Icons.delete),
                  onPressed: () => _onDeleteAgent(agent),
                ),
              ),
              onTap: () => _onEditAgent(agent),
            ),
          );
        },
      ),
      floatingActionButton: Semantics(
        button: true,
        label: 'Add agent',
        child: FloatingActionButton(
          onPressed: _onAddAgent,
          tooltip: 'Add agent',
          child: const Icon(Icons.add),
        ),
      ),
    );
  }
}

class _AgentEditorDialog extends StatefulWidget {
  const _AgentEditorDialog({
    required this.catalog,
    required this.providers,
    this.agent,
  });

  final CatalogClient catalog;
  final List<Provider> providers;
  final Agent? agent;

  @override
  State<_AgentEditorDialog> createState() => _AgentEditorDialogState();
}

class _AgentEditorDialogState extends State<_AgentEditorDialog> {
  late final TextEditingController _name;
  late final TextEditingController _description;
  late final TextEditingController _temperature;
  late final TextEditingController _maxTokens;
  late final TextEditingController _topP;
  late final TextEditingController _topK;
  late final TextEditingController _minP;
  late final TextEditingController _repetitionPenalty;
  late final TextEditingController _presencePenalty;
  String? _providerId;
  String? _defaultModel;
  String? _reasoningEffort;
  bool? _enableThinking;
  String? _error;
  bool _saving = false;

  bool get _isCreate => widget.agent == null;

  Provider? get _selectedProvider {
    final id = _providerId;
    if (id == null) {
      return null;
    }
    for (final provider in widget.providers) {
      if (provider.id == id) {
        return provider;
      }
    }
    return null;
  }

  bool get _isUnsloth => _selectedProvider?.type == providerTypeUnslothStudio;

  @override
  void initState() {
    super.initState();
    final agent = widget.agent;
    _name = TextEditingController(text: agent?.name ?? '');
    _description = TextEditingController(text: agent?.description ?? '');
    _providerId = agent?.providerId;
    _defaultModel = agent?.defaultModel;
    final inference = _inferenceMap(agent?.settings);
    _temperature = TextEditingController(
      text: _numText(inference['temperature']),
    );
    _maxTokens = TextEditingController(text: _numText(inference['maxTokens']));
    _topP = TextEditingController(text: _numText(inference['topP']));
    _topK = TextEditingController(text: _numText(inference['topK']));
    _minP = TextEditingController(text: _numText(inference['minP']));
    _repetitionPenalty = TextEditingController(
      text: _numText(inference['repetitionPenalty']),
    );
    _presencePenalty = TextEditingController(
      text: _numText(inference['presencePenalty']),
    );
    final effort = inference['reasoningEffort'];
    _reasoningEffort = effort is String ? effort : null;
    final thinking = inference['enableThinking'];
    _enableThinking = thinking is bool ? thinking : null;
  }

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    _temperature.dispose();
    _maxTokens.dispose();
    _topP.dispose();
    _topK.dispose();
    _minP.dispose();
    _repetitionPenalty.dispose();
    _presencePenalty.dispose();
    super.dispose();
  }

  Map<String, dynamic> _inferenceMap(Map<String, dynamic>? settings) {
    final raw = settings?['inference'];
    if (raw is Map) {
      return Map<String, dynamic>.from(raw);
    }
    return const {};
  }

  String _numText(Object? value) {
    if (value == null) {
      return '';
    }
    return '$value';
  }

  List<ModelInfo> get _models => _selectedProvider?.models ?? const [];

  bool get _canSubmit {
    return !_saving &&
        _name.text.trim().isNotEmpty &&
        _providerId != null &&
        _defaultModel != null;
  }

  void _onSubmit() {
    unawaited(
      _submit().catchError((Object e, StackTrace s) {
        AppLog.record('agent submit: $e', s);
      }),
    );
  }

  Map<String, dynamic>? _buildInferencePatch() {
    final patch = <String, dynamic>{};
    void putDouble(String key, TextEditingController c) {
      final raw = c.text.trim();
      if (raw.isEmpty) {
        patch[key] = null;
        return;
      }
      final value = double.tryParse(raw);
      if (value == null) {
        throw FormatException('Invalid number for $key');
      }
      patch[key] = value;
    }

    void putInt(String key, TextEditingController c) {
      final raw = c.text.trim();
      if (raw.isEmpty) {
        patch[key] = null;
        return;
      }
      final value = int.tryParse(raw);
      if (value == null) {
        throw FormatException('Invalid integer for $key');
      }
      patch[key] = value;
    }

    putDouble('temperature', _temperature);
    putInt('maxTokens', _maxTokens);
    patch['reasoningEffort'] = _reasoningEffort;

    if (_isUnsloth) {
      putDouble('topP', _topP);
      putInt('topK', _topK);
      putDouble('minP', _minP);
      putDouble('repetitionPenalty', _repetitionPenalty);
      putDouble('presencePenalty', _presencePenalty);
      patch['enableThinking'] = _enableThinking;
    }

    final hasValue = patch.values.any((v) => v != null);
    final clearing =
        !_isCreate && _inferenceMap(widget.agent?.settings).isNotEmpty;
    if (!hasValue && !clearing) {
      return null;
    }
    return patch;
  }

  Future<void> _submit() async {
    final providerId = _providerId;
    final defaultModel = _defaultModel;
    if (providerId == null || defaultModel == null) {
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final inference = _buildInferencePatch();
      if (_isCreate) {
        final created = await widget.catalog.createAgent(
          name: _name.text.trim(),
          description: _description.text.trim(),
          providerId: providerId,
          defaultModel: defaultModel,
        );
        if (inference != null) {
          await widget.catalog.updateAgent(
            created.id,
            settings: {'inference': inference},
          );
        }
      } else {
        await widget.catalog.updateAgent(
          widget.agent!.id,
          name: _name.text.trim(),
          description: _description.text.trim(),
          providerId: providerId,
          defaultModel: defaultModel,
          settings: inference == null ? null : {'inference': inference},
        );
      }
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(true);
    } on FormatException catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.message;
        _saving = false;
      });
    } on Object catch (e, s) {
      AppLog.record('agent submit failed: $e', s);
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
      title: Text(_isCreate ? 'Add agent' : 'Edit agent'),
      content: SizedBox(
        width: _isCreate ? 420 : 640,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              TextField(
                controller: _name,
                decoration: const InputDecoration(labelText: 'Name'),
                onChanged: (_) => setState(() {}),
              ),
              TextField(
                controller: _description,
                decoration: const InputDecoration(labelText: 'Description'),
              ),
              DropdownButtonFormField<String>(
                key: const Key('agent-provider'),
                initialValue: _providerId,
                decoration: const InputDecoration(labelText: 'Provider'),
                items: [
                  for (final provider in widget.providers)
                    DropdownMenuItem(
                      value: provider.id,
                      child: Text(provider.name),
                    ),
                ],
                onChanged: (value) {
                  setState(() {
                    _providerId = value;
                    _defaultModel = null;
                  });
                },
              ),
              DropdownButtonFormField<String>(
                key: const Key('agent-model'),
                initialValue: _defaultModel,
                decoration: const InputDecoration(labelText: 'Model'),
                items: [
                  for (final model in _models)
                    DropdownMenuItem(value: model.id, child: Text(model.name)),
                ],
                onChanged: (value) {
                  setState(() {
                    _defaultModel = value;
                  });
                },
              ),
              ExpansionTile(
                key: const Key('agent-inference'),
                title: const Text('Inference'),
                subtitle: const Text(
                  'Optional generation defaults for this agent',
                ),
                initiallyExpanded: true,
                children: [
                  TextField(
                    key: const Key('inference-temperature'),
                    controller: _temperature,
                    decoration: const InputDecoration(
                      labelText: 'Temperature',
                      hintText: '0–2, empty = provider default',
                    ),
                    keyboardType: const TextInputType.numberWithOptions(
                      decimal: true,
                    ),
                  ),
                  TextField(
                    key: const Key('inference-max-tokens'),
                    controller: _maxTokens,
                    decoration: const InputDecoration(
                      labelText: 'Max tokens',
                      hintText: 'Empty = provider default',
                    ),
                    keyboardType: TextInputType.number,
                  ),
                  DropdownButtonFormField<String?>(
                    key: const Key('inference-reasoning-effort'),
                    initialValue: _reasoningEffort,
                    decoration: const InputDecoration(
                      labelText: 'Reasoning effort',
                    ),
                    items: const [
                      DropdownMenuItem<String?>(
                        value: null,
                        child: Text('Default'),
                      ),
                      DropdownMenuItem(value: 'low', child: Text('Low')),
                      DropdownMenuItem(value: 'medium', child: Text('Medium')),
                      DropdownMenuItem(value: 'high', child: Text('High')),
                      DropdownMenuItem(value: 'xhigh', child: Text('XHigh')),
                      DropdownMenuItem(value: 'max', child: Text('Max')),
                    ],
                    onChanged: (value) {
                      setState(() => _reasoningEffort = value);
                    },
                  ),
                  if (_isUnsloth) ...[
                    TextField(
                      key: const Key('inference-top-p'),
                      controller: _topP,
                      decoration: const InputDecoration(labelText: 'Top P'),
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                    ),
                    TextField(
                      key: const Key('inference-top-k'),
                      controller: _topK,
                      decoration: const InputDecoration(labelText: 'Top K'),
                      keyboardType: TextInputType.number,
                    ),
                    TextField(
                      key: const Key('inference-min-p'),
                      controller: _minP,
                      decoration: const InputDecoration(labelText: 'Min P'),
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                    ),
                    TextField(
                      key: const Key('inference-repetition-penalty'),
                      controller: _repetitionPenalty,
                      decoration: const InputDecoration(
                        labelText: 'Repetition penalty',
                      ),
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                    ),
                    TextField(
                      key: const Key('inference-presence-penalty'),
                      controller: _presencePenalty,
                      decoration: const InputDecoration(
                        labelText: 'Presence penalty',
                      ),
                      keyboardType: const TextInputType.numberWithOptions(
                        decimal: true,
                      ),
                    ),
                    DropdownButtonFormField<bool?>(
                      key: const Key('inference-enable-thinking'),
                      initialValue: _enableThinking,
                      decoration: const InputDecoration(
                        labelText: 'Enable thinking',
                      ),
                      items: const [
                        DropdownMenuItem<bool?>(
                          value: null,
                          child: Text('Default'),
                        ),
                        DropdownMenuItem(value: true, child: Text('On')),
                        DropdownMenuItem(value: false, child: Text('Off')),
                      ],
                      onChanged: (value) {
                        setState(() => _enableThinking = value);
                      },
                    ),
                  ],
                ],
              ),
              const _ComingSoonTile(title: 'Tools'),
              const _ComingSoonTile(title: 'MCP'),
              const _ComingSoonTile(title: 'Memory'),
              if (_error != null) Text(_error!),
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
          onPressed: _canSubmit ? _onSubmit : null,
          child: Text(_isCreate ? 'Create' : 'Save'),
        ),
      ],
    );
  }
}

class _ComingSoonTile extends StatelessWidget {
  const _ComingSoonTile({required this.title});

  final String title;

  @override
  Widget build(BuildContext context) {
    return ExpansionTile(
      title: Text(title),
      subtitle: const Text('Coming soon'),
      enabled: false,
      children: const [],
    );
  }
}
