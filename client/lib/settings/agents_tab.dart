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
  List<Agent> _agents = [];
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
      final agents = await widget.catalog.listAgents();
      final providers = await widget.catalog.listProviders();
      if (!mounted) {
        return;
      }
      setState(() {
        _agents = agents;
        _providers = providers;
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
        onPressed: () => _openEditor(),
        tooltip: 'Add agent',
        child: const Icon(Icons.add),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    final list = _agents.isEmpty
        ? const Center(child: Text('No agents'))
        : ListView.builder(
            itemCount: _agents.length,
            itemBuilder: (context, index) {
              final agent = _agents[index];
              return ListTile(
                title: Text(agent.name),
                subtitle: Text(
                  !agent.isComplete
                      ? 'Needs provider'
                      : (agent.description.isEmpty
                            ? (agent.defaultModel ?? '')
                            : agent.description),
                ),
                trailing: IconButton(
                  key: Key('delete-agent-${agent.id}'),
                  tooltip: 'Delete agent',
                  icon: const Icon(Icons.delete),
                  onPressed: () => _confirmDelete(agent),
                ),
                onTap: () => _openEditor(agent: agent),
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
  String? _providerId;
  String? _defaultModel;
  String? _error;
  bool _saving = false;

  bool get _isCreate => widget.agent == null;

  @override
  void initState() {
    super.initState();
    final agent = widget.agent;
    _name = TextEditingController(text: agent?.name ?? '');
    _description = TextEditingController(text: agent?.description ?? '');
    _providerId = agent?.providerId;
    _defaultModel = agent?.defaultModel;
  }

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    super.dispose();
  }

  List<ModelInfo> get _models {
    final id = _providerId;
    if (id == null) {
      return const [];
    }
    for (final provider in widget.providers) {
      if (provider.id == id) {
        return provider.models;
      }
    }
    return const [];
  }

  bool get _canSubmit {
    return !_saving &&
        _name.text.trim().isNotEmpty &&
        _providerId != null &&
        _defaultModel != null;
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
      if (_isCreate) {
        await widget.catalog.createAgent(
          name: _name.text.trim(),
          description: _description.text.trim(),
          providerId: providerId,
          defaultModel: defaultModel,
        );
      } else {
        await widget.catalog.updateAgent(
          widget.agent!.id,
          name: _name.text.trim(),
          description: _description.text.trim(),
          providerId: providerId,
          defaultModel: defaultModel,
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
    return AlertDialog(
      title: Text(_isCreate ? 'Add agent' : 'Edit agent'),
      content: SizedBox(
        width: _isCreate ? 420 : 640,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
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
          onPressed: _canSubmit ? _submit : null,
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
