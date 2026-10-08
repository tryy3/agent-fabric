import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:agent_fabric_client/core/settings_load_state.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../ui/model_specs_widgets.dart';
import 'inference_param_row.dart';
import 'permissions_editor.dart';

class AssistantsTab extends StatefulWidget {
  const AssistantsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<AssistantsTab> createState() => _AssistantsTabState();
}

class _AssistantsTabState extends State<AssistantsTab> {
  SettingsLoadState<Assistant> _state = const SettingsLoading();
  List<InferenceConnection> _inferenceConnections = const [];
  List<ToolIntegration> _toolIntegrations = const [];

  @override
  void initState() {
    super.initState();
    _startReload();
  }

  void _startReload() {
    unawaited(
      _reload().catchError((Object e, StackTrace s) {
        AppLog.record('assistants reload: $e', s);
      }),
    );
  }

  void _onAddAssistant() {
    unawaited(
      _openEditor().catchError((Object e, StackTrace s) {
        AppLog.record('assistants open editor: $e', s);
      }),
    );
  }

  void _onEditAssistant(Assistant assistant) {
    unawaited(
      _openEditor(assistant: assistant).catchError((Object e, StackTrace s) {
        AppLog.record('assistants edit: $e', s);
      }),
    );
  }

  void _onDeleteAssistant(Assistant assistant) {
    unawaited(
      _confirmDelete(assistant).catchError((Object e, StackTrace s) {
        AppLog.record('assistants delete: $e', s);
      }),
    );
  }

  Future<void> _reload() async {
    setState(() => _state = const SettingsLoading());
    try {
      final assistants = await widget.catalog.listAssistants();
      final providers = await widget.catalog.listInferenceConnections();
      final tools = await widget.catalog.listToolIntegrations();
      if (!mounted) {
        return;
      }
      setState(() {
        _inferenceConnections = providers;
        _toolIntegrations = tools;
        _state = SettingsReady(assistants);
      });
    } on Object catch (e, s) {
      AppLog.record('assistants reload failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _state = SettingsFailed(operatorFailureFrom(e)));
    }
  }

  Future<void> _openEditor({Assistant? assistant}) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) => _AssistantEditorDialog(
        catalog: widget.catalog,
        inferenceConnections: _inferenceConnections,
        toolIntegrations: _toolIntegrations,
        assistant: assistant,
      ),
    );
    if (saved == true) {
      await _reload();
    }
  }

  Future<void> _confirmDelete(Assistant assistant) async {
    try {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) {
          return AlertDialog(
            title: const Text('Delete assistant?'),
            content: Text('Delete ${assistant.name}?'),
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
      await widget.catalog.deleteAssistant(assistant.id);
      await _reload();
    } on Object catch (e, s) {
      AppLog.record('assistants delete failed: $e', s);
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
      body: SettingsLoadBody<Assistant>(
        state: _state,
        emptyLabel: 'No assistants',
        onRetry: _startReload,
        itemBuilder: (context, assistant) {
          return Semantics(
            button: true,
            label: 'Assistant ${assistant.name}',
            child: ListTile(
              title: Text(assistant.name),
              subtitle: Text(
                !assistant.isComplete
                    ? 'Needs connection'
                    : (assistant.description.isEmpty
                          ? (assistant.defaultModel ?? '')
                          : assistant.description),
              ),
              trailing: Semantics(
                button: true,
                label: 'Delete assistant ${assistant.name}',
                child: IconButton(
                  key: Key('delete-agent-${assistant.id}'),
                  tooltip: 'Delete assistant',
                  icon: const Icon(Icons.delete),
                  onPressed: () => _onDeleteAssistant(assistant),
                ),
              ),
              onTap: () => _onEditAssistant(assistant),
            ),
          );
        },
      ),
      floatingActionButton: Semantics(
        button: true,
        label: 'Add assistant',
        child: FloatingActionButton(
          onPressed: _onAddAssistant,
          tooltip: 'Add assistant',
          child: const Icon(Icons.add),
        ),
      ),
    );
  }
}

class _AssistantEditorDialog extends StatefulWidget {
  const _AssistantEditorDialog({
    required this.catalog,
    required this.inferenceConnections,
    required this.toolIntegrations,
    this.assistant,
  });

  final CatalogClient catalog;
  final List<InferenceConnection> inferenceConnections;
  final List<ToolIntegration> toolIntegrations;
  final Assistant? assistant;

  @override
  State<_AssistantEditorDialog> createState() => _AssistantEditorDialogState();
}

class _AssistantEditorDialogState extends State<_AssistantEditorDialog> {
  late final TextEditingController _name;
  late final TextEditingController _description;
  late final TextEditingController _instructions;
  late final TextEditingController _temperature;
  late final TextEditingController _maxTokens;
  late final TextEditingController _topP;
  late final TextEditingController _topK;
  late final TextEditingController _minP;
  late final TextEditingController _repetitionPenalty;
  late final TextEditingController _presencePenalty;
  late final TextEditingController _frequencyPenalty;
  String? _inferenceConnectionId;
  String? _defaultModel;
  String? _reasoningEffort;
  bool? _enableThinking;
  String? _thinkingType;
  String _webSearchMode = 'inherit';
  String? _webSearchIntegrationId;
  String _fetchPageMode = 'inherit';
  String? _fetchPageIntegrationId;
  late final List<PermissionRuleDraft> _permissionRules;
  late final PermissionScorerDraft _fastScorer;
  late final PermissionScorerDraft _deepScorer;
  late final PermissionScorerTuning _scorerTuning;
  String? _error;
  bool _saving = false;

  bool get _isCreate => widget.assistant == null;

  InferenceConnection? get _selectedInferenceConnection {
    final id = _inferenceConnectionId;
    if (id == null) {
      return null;
    }
    for (final provider in widget.inferenceConnections) {
      if (provider.id == id) {
        return provider;
      }
    }
    return null;
  }

  String? get _selectedType => _selectedInferenceConnection?.type;

  bool get _isUnsloth => _selectedType == providerTypeUnslothStudio;

  bool get _isBerget => _selectedType == providerTypeBergetAI;

  bool get _hasSamplerExtras =>
      _selectedType != null && supportsSamplerExtras(_selectedType!);

  @override
  void initState() {
    super.initState();
    final assistant = widget.assistant;
    _name = TextEditingController(text: assistant?.name ?? '');
    _description = TextEditingController(text: assistant?.description ?? '');
    _instructions = TextEditingController(text: assistant?.instructions ?? '');
    _inferenceConnectionId = assistant?.inferenceConnectionId;
    _defaultModel = assistant?.defaultModel;
    final inference = _inferenceMap(assistant?.settings);
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
    _frequencyPenalty = TextEditingController(
      text: _numText(inference['frequencyPenalty']),
    );
    final effort = inference['reasoningEffort'];
    _reasoningEffort = effort is String ? effort : null;
    final thinking = inference['enableThinking'];
    _enableThinking = thinking is bool ? thinking : null;
    final thinkingType = inference['thinkingType'];
    _thinkingType = thinkingType is String ? thinkingType : null;
    _permissionRules = permissionRuleDrafts(assistant?.settings);
    final permissions = assistant?.settings['permissions'];
    final scorers = permissions is Map ? permissions['scorers'] : null;
    _fastScorer = PermissionScorerDraft.fromJson(
      scorers is Map ? scorers['fast'] : null,
    );
    _deepScorer = PermissionScorerDraft.fromJson(
      scorers is Map ? scorers['deep'] : null,
    );
    _scorerTuning = PermissionScorerTuning.fromJson(scorers);
    final bindings = _toolBindingsMap(assistant?.settings);
    final webSearch = bindings['webSearch'];
    if (webSearch is Map) {
      _webSearchMode = '${webSearch['mode'] ?? 'inherit'}';
      final id = webSearch['integrationId'];
      _webSearchIntegrationId = id is String ? id : null;
    }
    final fetchPage = bindings['fetchPage'];
    if (fetchPage is Map) {
      _fetchPageMode = '${fetchPage['mode'] ?? 'inherit'}';
      final id = fetchPage['integrationId'];
      _fetchPageIntegrationId = id is String ? id : null;
    }
  }

  Map<String, dynamic> _toolBindingsMap(Map<String, dynamic>? settings) {
    final raw = settings?['toolBindings'];
    if (raw is Map<String, dynamic>) {
      return raw;
    }
    if (raw is Map) {
      return Map<String, dynamic>.from(raw);
    }
    return const {};
  }

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    _instructions.dispose();
    _temperature.dispose();
    _maxTokens.dispose();
    _topP.dispose();
    _topK.dispose();
    _minP.dispose();
    _repetitionPenalty.dispose();
    _presencePenalty.dispose();
    _frequencyPenalty.dispose();
    for (final rule in _permissionRules) {
      rule.dispose();
    }
    _fastScorer.dispose();
    _deepScorer.dispose();
    _scorerTuning.dispose();
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

  List<ModelInfo> get _models =>
      _selectedInferenceConnection?.models ?? const [];

  bool get _canSubmit {
    return !_saving &&
        _name.text.trim().isNotEmpty &&
        _inferenceConnectionId != null &&
        _defaultModel != null;
  }

  void _onSubmit() {
    unawaited(
      _submit().catchError((Object e, StackTrace s) {
        AppLog.record('assistant submit: $e', s);
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

    if (_hasSamplerExtras) {
      putDouble('topP', _topP);
      putInt('topK', _topK);
      putDouble('minP', _minP);
      putDouble('repetitionPenalty', _repetitionPenalty);
      putDouble('presencePenalty', _presencePenalty);
    }
    if (_isBerget) {
      putDouble('frequencyPenalty', _frequencyPenalty);
      patch['thinkingType'] = _thinkingType;
    }
    if (_isUnsloth) {
      patch['enableThinking'] = _enableThinking;
    }

    final hasValue = patch.values.any((v) => v != null);
    final clearing =
        !_isCreate && _inferenceMap(widget.assistant?.settings).isNotEmpty;
    if (!hasValue && !clearing) {
      return null;
    }
    return patch;
  }

  Map<String, dynamic> _toolBindingsPatch() {
    Map<String, dynamic> one(String mode, String? id) {
      if (mode == 'integration') {
        return {'mode': mode, 'integrationId': id};
      }
      return {'mode': mode};
    }

    return {
      'webSearch': one(_webSearchMode, _webSearchIntegrationId),
      'fetchPage': one(_fetchPageMode, _fetchPageIntegrationId),
    };
  }

  Future<void> _submit() async {
    final providerId = _inferenceConnectionId;
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
      final settings = <String, dynamic>{
        'toolBindings': _toolBindingsPatch(),
        'permissions': permissionsPatch(
          rules: _permissionRules,
          fast: _fastScorer,
          deep: _deepScorer,
          tuning: _scorerTuning,
        ),
        if (inference != null) 'inference': inference,
      };
      if (_isCreate) {
        final created = await widget.catalog.createAssistant(
          name: _name.text.trim(),
          description: _description.text.trim(),
          instructions: _instructions.text,
          inferenceConnectionId: providerId,
          defaultModel: defaultModel,
        );
        await widget.catalog.updateAssistant(created.id, settings: settings);
      } else {
        await widget.catalog.updateAssistant(
          widget.assistant!.id,
          name: _name.text.trim(),
          description: _description.text.trim(),
          instructions: _instructions.text,
          inferenceConnectionId: providerId,
          defaultModel: defaultModel,
          settings: settings,
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
      AppLog.record('assistant submit failed: $e', s);
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
      title: Text(_isCreate ? 'Add assistant' : 'Edit assistant'),
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
              TextField(
                key: const Key('assistant-instructions'),
                controller: _instructions,
                minLines: 4,
                maxLines: 12,
                decoration: const InputDecoration(
                  labelText: 'Assistant instructions',
                  alignLabelWithHint: true,
                  helperText:
                      'Defines this assistant\'s role, expertise, priorities '
                      'and communication style. Supports instruction '
                      'variables (see Settings → Instructions).',
                  hintText:
                      'Role, expertise, priorities, and communication style',
                ),
              ),
              DropdownButtonFormField<String>(
                key: const Key('agent-provider'),
                initialValue: _inferenceConnectionId,
                decoration: const InputDecoration(labelText: 'Connection'),
                items: [
                  for (final provider in widget.inferenceConnections)
                    DropdownMenuItem(
                      value: provider.id,
                      child: Text(provider.name),
                    ),
                ],
                onChanged: (value) {
                  setState(() {
                    _inferenceConnectionId = value;
                    _defaultModel = null;
                  });
                },
              ),
              DropdownButtonFormField<String>(
                key: const Key('agent-model'),
                initialValue: _defaultModel,
                decoration: const InputDecoration(labelText: 'Model'),
                isExpanded: true,
                selectedItemBuilder: (context) => [
                  for (final model in _models)
                    Text(model.name, overflow: TextOverflow.ellipsis),
                ],
                items: [
                  for (final model in _models)
                    DropdownMenuItem(
                      value: model.id,
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(model.name, overflow: TextOverflow.ellipsis),
                          ModelSpecsSummary(specs: model.specs),
                        ],
                      ),
                    ),
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
                  'Optional generation defaults for this assistant',
                ),
                initiallyExpanded: true,
                children: [
                  InferenceParamRow(
                    fieldKey: const Key('inference-temperature'),
                    label: 'Temperature',
                    tooltip:
                        'Controls randomness. Lower is more deterministic; '
                        'higher is more creative. Range 0–2. Empty uses the '
                        'provider default.',
                    controller: _temperature,
                    min: 0,
                    max: 2,
                    unsetDisplay: 1,
                    divisions: 200,
                    onChanged: () => setState(() {}),
                  ),
                  InferenceParamRow(
                    fieldKey: const Key('inference-max-tokens'),
                    label: 'Max tokens',
                    tooltip:
                        'Maximum tokens to generate. Empty leaves the limit '
                        'to the provider or model default.',
                    controller: _maxTokens,
                    min: 1,
                    max: 32768,
                    unsetDisplay: 32768,
                    integer: true,
                    divisions: 128,
                    onChanged: () => setState(() {}),
                  ),
                  InferenceLabeledControl(
                    label: 'Reasoning effort',
                    tooltip:
                        'How hard reasoning / thinking models should work '
                        '(none → max). Mapped per provider wire API. Default '
                        'leaves effort unset. On some Berget models that always '
                        'think, none only lowers the budget.',
                    child: DropdownButtonFormField<String?>(
                      key: const Key('inference-reasoning-effort'),
                      initialValue: _reasoningEffort,
                      decoration: const InputDecoration(
                        isDense: true,
                        border: OutlineInputBorder(),
                      ),
                      items: const [
                        DropdownMenuItem<String?>(
                          value: null,
                          child: Text('Default'),
                        ),
                        DropdownMenuItem(value: 'none', child: Text('None')),
                        DropdownMenuItem(
                          value: 'minimal',
                          child: Text('Minimal'),
                        ),
                        DropdownMenuItem(value: 'low', child: Text('Low')),
                        DropdownMenuItem(
                          value: 'medium',
                          child: Text('Medium'),
                        ),
                        DropdownMenuItem(value: 'high', child: Text('High')),
                        DropdownMenuItem(value: 'xhigh', child: Text('XHigh')),
                        DropdownMenuItem(value: 'max', child: Text('Max')),
                      ],
                      onChanged: (value) {
                        setState(() => _reasoningEffort = value);
                      },
                    ),
                  ),
                  if (_hasSamplerExtras) ...[
                    InferenceParamRow(
                      fieldKey: const Key('inference-top-p'),
                      label: 'Top P',
                      tooltip:
                          'Nucleus sampling: keep the smallest set of tokens '
                          'whose cumulative probability is at least P. '
                          'Range 0–1.',
                      controller: _topP,
                      min: 0,
                      max: 1,
                      unsetDisplay: 0.95,
                      divisions: 100,
                      onChanged: () => setState(() {}),
                    ),
                    InferenceParamRow(
                      fieldKey: const Key('inference-top-k'),
                      label: 'Top K',
                      tooltip:
                          'Only sample from the K most likely tokens. '
                          'Typical local values are 20–64. Slider covers 0–100; '
                          'type a higher value (up to 1000) for rare cases.',
                      controller: _topK,
                      min: 0,
                      max: 100,
                      unsetDisplay: 20,
                      integer: true,
                      divisions: 100,
                      onChanged: () => setState(() {}),
                    ),
                    InferenceParamRow(
                      fieldKey: const Key('inference-min-p'),
                      label: 'Min P',
                      tooltip:
                          'Drop tokens below this fraction of the top token\'s '
                          'probability. Often clearer than Top P at higher '
                          'temperature. Range 0–1.',
                      controller: _minP,
                      min: 0,
                      max: 1,
                      unsetDisplay: 0.05,
                      divisions: 100,
                      onChanged: () => setState(() {}),
                    ),
                    InferenceParamRow(
                      fieldKey: const Key('inference-repetition-penalty'),
                      label: 'Repetition penalty',
                      tooltip:
                          'Penalizes repeating tokens. 1.0 is off; higher '
                          'values discourage repetition. Range 1–2.',
                      controller: _repetitionPenalty,
                      min: 1,
                      max: 2,
                      unsetDisplay: 1,
                      divisions: 100,
                      onChanged: () => setState(() {}),
                    ),
                    InferenceParamRow(
                      fieldKey: const Key('inference-presence-penalty'),
                      label: 'Presence penalty',
                      tooltip: _isBerget
                          ? 'Encourages introducing new topics. 0 is off. '
                                'Range −2–2.'
                          : 'Encourages introducing new topics. 0 is off. '
                                'Range 0–2.',
                      controller: _presencePenalty,
                      min: _isBerget ? -2 : 0,
                      max: 2,
                      unsetDisplay: 0,
                      divisions: _isBerget ? 400 : 200,
                      onChanged: () => setState(() {}),
                    ),
                  ],
                  if (_isBerget) ...[
                    InferenceParamRow(
                      fieldKey: const Key('inference-frequency-penalty'),
                      label: 'Frequency penalty',
                      tooltip:
                          'Penalizes tokens proportional to how often they '
                          'already appeared. 0 is off. Range −2–2.',
                      controller: _frequencyPenalty,
                      min: -2,
                      max: 2,
                      unsetDisplay: 0,
                      divisions: 400,
                      onChanged: () => setState(() {}),
                    ),
                    InferenceLabeledControl(
                      label: 'Thinking',
                      tooltip:
                          'Moonshot/Kimi K2 CoT control (disabled / enabled / '
                          'adaptive). Not supported on always-think models '
                          '(e.g. Kimi K3); use reasoning effort instead. '
                          'Default leaves the server setting unchanged.',
                      child: DropdownButtonFormField<String?>(
                        key: const Key('inference-thinking-type'),
                        initialValue: _thinkingType,
                        decoration: const InputDecoration(
                          isDense: true,
                          border: OutlineInputBorder(),
                        ),
                        items: const [
                          DropdownMenuItem<String?>(
                            value: null,
                            child: Text('Default'),
                          ),
                          DropdownMenuItem(
                            value: 'disabled',
                            child: Text('Disabled'),
                          ),
                          DropdownMenuItem(
                            value: 'enabled',
                            child: Text('Enabled'),
                          ),
                          DropdownMenuItem(
                            value: 'adaptive',
                            child: Text('Adaptive'),
                          ),
                        ],
                        onChanged: (value) {
                          setState(() => _thinkingType = value);
                        },
                      ),
                    ),
                  ],
                  if (_isUnsloth)
                    InferenceLabeledControl(
                      label: 'Enable thinking',
                      tooltip:
                          'Unsloth thinking / reasoning mode. Default leaves '
                          'the server setting unchanged.',
                      child: DropdownButtonFormField<bool?>(
                        key: const Key('inference-enable-thinking'),
                        initialValue: _enableThinking,
                        decoration: const InputDecoration(
                          isDense: true,
                          border: OutlineInputBorder(),
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
                    ),
                ],
              ),
              const _ComingSoonTile(title: 'MCP'),
              const _ComingSoonTile(title: 'Memory'),
              ExpansionTile(
                key: const Key('agent-tool-bindings'),
                title: const Text('Web tool bindings'),
                subtitle: const Text(
                  'Inherit plane defaults, disable, or pick an integration',
                ),
                children: [
                  _BindingSelector(
                    key: const Key('binding-web-search'),
                    label: 'web_search',
                    mode: _webSearchMode,
                    integrationId: _webSearchIntegrationId,
                    integrations: widget.toolIntegrations
                        .where((t) => t.capabilities.contains('web_search'))
                        .toList(),
                    onModeChanged: (m) => setState(() => _webSearchMode = m),
                    onIntegrationChanged: (id) =>
                        setState(() => _webSearchIntegrationId = id),
                  ),
                  const SizedBox(height: 8),
                  _BindingSelector(
                    key: const Key('binding-fetch-page'),
                    label: 'fetch_page',
                    mode: _fetchPageMode,
                    integrationId: _fetchPageIntegrationId,
                    integrations: widget.toolIntegrations
                        .where((t) => t.capabilities.contains('fetch_page'))
                        .toList(),
                    onModeChanged: (m) => setState(() => _fetchPageMode = m),
                    onIntegrationChanged: (id) =>
                        setState(() => _fetchPageIntegrationId = id),
                  ),
                ],
              ),
              ExpansionTile(
                key: const Key('agent-permissions'),
                title: const Text('Permissions'),
                subtitle: const Text(
                  'Rules that always allow, ask or deny, and gate scorers',
                ),
                childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
                children: [
                  PermissionRulesEditor(
                    rules: _permissionRules,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 12),
                  PermissionScorerPicker(
                    label: 'Fast scorer',
                    help:
                        'Optional System One model that scores calls the '
                        'rules do not settle. Jev is recommended; other '
                        'System One models scored poorly in the gate '
                        'benchmark.',
                    scorer: _fastScorer,
                    connections: widget.inferenceConnections,
                    withStrategy: true,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 12),
                  PermissionScorerPicker(
                    label: 'Deep scorer',
                    help:
                        'Optional chat model, asked when there is no fast '
                        'scorer or it is unsure. It sees your request and may '
                        'lower a score by a limited amount.',
                    scorer: _deepScorer,
                    connections: widget.inferenceConnections,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 12),
                  MaxLowerField(
                    tuning: _scorerTuning,
                    onChanged: () => setState(() {}),
                  ),
                ],
              ),
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

class _BindingSelector extends StatelessWidget {
  const _BindingSelector({
    super.key,
    required this.label,
    required this.mode,
    required this.integrationId,
    required this.integrations,
    required this.onModeChanged,
    required this.onIntegrationChanged,
  });

  final String label;
  final String mode;
  final String? integrationId;
  final List<ToolIntegration> integrations;
  final ValueChanged<String> onModeChanged;
  final ValueChanged<String?> onIntegrationChanged;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          DropdownButtonFormField<String>(
            initialValue: mode,
            decoration: InputDecoration(
              labelText: '$label binding',
              border: const OutlineInputBorder(),
            ),
            items: const [
              DropdownMenuItem(
                value: 'inherit',
                child: Text('Inherit default'),
              ),
              DropdownMenuItem(value: 'disabled', child: Text('Disabled')),
              DropdownMenuItem(
                value: 'integration',
                child: Text('Specific integration'),
              ),
            ],
            onChanged: (v) {
              if (v != null) {
                onModeChanged(v);
              }
            },
          ),
          if (mode == 'integration') ...[
            const SizedBox(height: 8),
            DropdownButtonFormField<String>(
              initialValue: integrationId,
              decoration: const InputDecoration(
                labelText: 'Integration',
                border: OutlineInputBorder(),
              ),
              items: [
                for (final ti in integrations)
                  DropdownMenuItem(value: ti.id, child: Text(ti.name)),
              ],
              onChanged: onIntegrationChanged,
            ),
          ],
        ],
      ),
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
