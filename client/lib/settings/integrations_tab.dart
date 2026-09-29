import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';

sealed class _IntegrationsLoadState {
  const _IntegrationsLoadState();
}

final class _IntegrationsLoading extends _IntegrationsLoadState {
  const _IntegrationsLoading();
}

final class _IntegrationsFailed extends _IntegrationsLoadState {
  const _IntegrationsFailed(this.message);

  final String message;
}

final class _IntegrationsReady extends _IntegrationsLoadState {
  const _IntegrationsReady({
    required this.apiKey,
    required this.accountId,
    required this.toolIntegrations,
    required this.webSearchIntegrationId,
    required this.fetchPageIntegrationId,
  });

  final String apiKey;
  final String accountId;
  final List<ToolIntegration> toolIntegrations;
  final String? webSearchIntegrationId;
  final String? fetchPageIntegrationId;
}

/// Plane-level third-party credentials and web tool integrations.
class IntegrationsTab extends StatefulWidget {
  const IntegrationsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<IntegrationsTab> createState() => _IntegrationsTabState();
}

class _IntegrationsTabState extends State<IntegrationsTab> {
  _IntegrationsLoadState _state = const _IntegrationsLoading();
  final _apiKeyController = TextEditingController();
  final _accountIdController = TextEditingController();
  var _saving = false;
  String? _saveError;
  String? _webSearchDefault;
  String? _fetchPageDefault;

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  @override
  void dispose() {
    _apiKeyController.dispose();
    _accountIdController.dispose();
    super.dispose();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('integrations load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() {
      _state = const _IntegrationsLoading();
      _saveError = null;
    });
    try {
      final settings = await widget.catalog.getSettings();
      final tools = await widget.catalog.listToolIntegrations();
      final netlify = settings.integrations['netlify'];
      var apiKey = '';
      var accountId = '';
      if (netlify is Map) {
        apiKey = '${netlify['apiKey'] ?? ''}';
        accountId = '${netlify['accountId'] ?? ''}';
      }
      if (!mounted) {
        return;
      }
      _apiKeyController.text = apiKey;
      _accountIdController.text = accountId;
      _webSearchDefault = settings.webSearchIntegrationId;
      _fetchPageDefault = settings.fetchPageIntegrationId;
      setState(() {
        _state = _IntegrationsReady(
          apiKey: apiKey,
          accountId: accountId,
          toolIntegrations: tools,
          webSearchIntegrationId: settings.webSearchIntegrationId,
          fetchPageIntegrationId: settings.fetchPageIntegrationId,
        );
      });
    } on Object catch (e, s) {
      AppLog.record('integrations load failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _state = _IntegrationsFailed('$e');
      });
    }
  }

  Future<void> _saveNetlify() async {
    setState(() {
      _saving = true;
      _saveError = null;
    });
    try {
      await widget.catalog.patchSettings(
        integrations: {
          'netlify': {
            'apiKey': _apiKeyController.text.trim(),
            'accountId': _accountIdController.text.trim(),
          },
        },
      );
      if (!mounted) {
        return;
      }
      setState(() => _saving = false);
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Netlify saved')));
    } on Object catch (e, s) {
      AppLog.record('integrations save failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _saving = false;
        _saveError = '$e';
      });
    }
  }

  Future<void> _saveDefaults() async {
    setState(() {
      _saving = true;
      _saveError = null;
    });
    try {
      await widget.catalog.patchSettings(
        webSearchIntegrationId: _webSearchDefault,
        fetchPageIntegrationId: _fetchPageDefault,
      );
      if (!mounted) {
        return;
      }
      setState(() => _saving = false);
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('Web tool defaults saved')));
      _startLoad();
    } on Object catch (e, s) {
      AppLog.record('web defaults save failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _saving = false;
        _saveError = '$e';
      });
    }
  }

  Future<void> _createTool() async {
    final created = await showDialog<bool>(
      context: context,
      builder: (context) => _ToolIntegrationDialog(catalog: widget.catalog),
    );
    if (created == true) {
      _startLoad();
    }
  }

  Future<void> _editTool(ToolIntegration ti) async {
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) =>
          _ToolIntegrationDialog(catalog: widget.catalog, existing: ti),
    );
    if (saved == true) {
      _startLoad();
    }
  }

  Future<void> _testTool(ToolIntegration ti) async {
    try {
      final result = await widget.catalog.testToolIntegration(ti.id);
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            result.ok ? 'Connection OK' : 'Failed: ${result.message}',
          ),
        ),
      );
      _startLoad();
    } on Object catch (e, s) {
      AppLog.record('tool integration test: $e', s);
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Test failed: $e')));
    }
  }

  Future<void> _deleteTool(ToolIntegration ti) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete integration?'),
        content: Text(
          'Remove ${ti.name}? Assistants using it will fail until rebound.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (ok != true) {
      return;
    }
    await widget.catalog.deleteToolIntegration(ti.id);
    _startLoad();
  }

  @override
  Widget build(BuildContext context) {
    return switch (_state) {
      _IntegrationsLoading() => const Center(
        child: CircularProgressIndicator(),
      ),
      _IntegrationsFailed(:final message) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(message),
            const SizedBox(height: 12),
            FilledButton(onPressed: _startLoad, child: const Text('Retry')),
          ],
        ),
      ),
      _IntegrationsReady(:final toolIntegrations) => ListView(
        key: const Key('integrations-tab'),
        padding: const EdgeInsets.all(24),
        children: [
          Text('Web tools', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          Text(
            'Plane-owned search and page-reading backends. Secrets are write-only '
            'and never returned after save.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 12),
          for (final ti in toolIntegrations)
            ListTile(
              key: Key('tool-integration-${ti.id}'),
              title: Text(ti.name),
              subtitle: Text(
                '${ti.kind} · ${ti.mode} · ${ti.enabled ? 'enabled' : 'disabled'}'
                '${ti.apiKeyConfigured ? ' · apiKey configured' : ''}'
                ' · health ${ti.healthStatus}',
              ),
              trailing: Wrap(
                spacing: 4,
                children: [
                  TextButton(
                    onPressed: () => unawaited(_testTool(ti)),
                    child: const Text('Test'),
                  ),
                  TextButton(
                    onPressed: () => unawaited(_editTool(ti)),
                    child: const Text('Edit'),
                  ),
                  TextButton(
                    onPressed: () => unawaited(_deleteTool(ti)),
                    child: const Text('Delete'),
                  ),
                ],
              ),
            ),
          Align(
            alignment: Alignment.centerLeft,
            child: OutlinedButton(
              key: const Key('tool-integration-add'),
              onPressed: () => unawaited(_createTool()),
              child: const Text('Add tool integration'),
            ),
          ),
          const SizedBox(height: 24),
          Text(
            'Plane defaults',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 8),
          DropdownButtonFormField<String?>(
            key: const Key('default-web-search'),
            initialValue: _webSearchDefault,
            decoration: const InputDecoration(
              labelText: 'Default web_search integration',
              border: OutlineInputBorder(),
            ),
            items: [
              const DropdownMenuItem<String?>(value: null, child: Text('None')),
              for (final ti in toolIntegrations.where(
                (t) => t.capabilities.contains('web_search'),
              ))
                DropdownMenuItem(value: ti.id, child: Text(ti.name)),
            ],
            onChanged: (v) => setState(() => _webSearchDefault = v),
          ),
          const SizedBox(height: 12),
          DropdownButtonFormField<String?>(
            key: const Key('default-fetch-page'),
            initialValue: _fetchPageDefault,
            decoration: const InputDecoration(
              labelText: 'Default fetch_page integration',
              border: OutlineInputBorder(),
            ),
            items: [
              const DropdownMenuItem<String?>(value: null, child: Text('None')),
              for (final ti in toolIntegrations.where(
                (t) => t.capabilities.contains('fetch_page'),
              ))
                DropdownMenuItem(value: ti.id, child: Text(ti.name)),
            ],
            onChanged: (v) => setState(() => _fetchPageDefault = v),
          ),
          const SizedBox(height: 12),
          Align(
            alignment: Alignment.centerLeft,
            child: FilledButton(
              key: const Key('web-defaults-save'),
              onPressed: _saving ? null : () => unawaited(_saveDefaults()),
              child: const Text('Save defaults'),
            ),
          ),
          const SizedBox(height: 32),
          Text('Netlify', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          Text(
            'Personal access token used by the control plane to publish '
            'project workspaces. The token never leaves the server.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 16),
          TextField(
            key: const Key('netlify-api-key'),
            controller: _apiKeyController,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'API key / personal access token',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            key: const Key('netlify-account-id'),
            controller: _accountIdController,
            decoration: const InputDecoration(
              labelText: 'Account / team ID (optional)',
              border: OutlineInputBorder(),
              helperText:
                  'Leave blank to create sites under your personal account.',
            ),
          ),
          if (_saveError != null) ...[
            const SizedBox(height: 12),
            Text(
              _saveError!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ],
          const SizedBox(height: 16),
          Align(
            alignment: Alignment.centerLeft,
            child: FilledButton(
              key: const Key('integrations-save'),
              onPressed: _saving
                  ? null
                  : () {
                      unawaited(
                        _saveNetlify().catchError((Object e, StackTrace s) {
                          AppLog.record('integrations save tap: $e', s);
                        }),
                      );
                    },
              child: _saving
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('Save Netlify'),
            ),
          ),
        ],
      ),
    };
  }
}

class _ToolIntegrationDialog extends StatefulWidget {
  const _ToolIntegrationDialog({required this.catalog, this.existing});

  final CatalogClient catalog;
  final ToolIntegration? existing;

  @override
  State<_ToolIntegrationDialog> createState() => _ToolIntegrationDialogState();
}

class _ToolIntegrationDialogState extends State<_ToolIntegrationDialog> {
  late final TextEditingController _name;
  late final TextEditingController _endpoint;
  late final TextEditingController _apiKey;
  late String _kind;
  late String _mode;
  var _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    final e = widget.existing;
    _name = TextEditingController(text: e?.name ?? '');
    _endpoint = TextEditingController(text: e?.endpoint ?? '');
    _apiKey = TextEditingController();
    _kind = e?.kind ?? 'searxng';
    _mode = e?.mode ?? (e?.kind == 'linkup' ? 'external' : 'bundled');
  }

  @override
  void dispose() {
    _name.dispose();
    _endpoint.dispose();
    _apiKey.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final secrets = <String, String?>{};
      final key = _apiKey.text.trim();
      if (key.isNotEmpty) {
        secrets['apiKey'] = key;
      }
      if (widget.existing == null) {
        await widget.catalog.createToolIntegration(
          name: _name.text.trim(),
          kind: _kind,
          mode: _mode,
          endpoint: _endpoint.text.trim(),
          secrets: secrets.isEmpty ? null : secrets,
        );
      } else {
        await widget.catalog.updateToolIntegration(
          widget.existing!.id,
          name: _name.text.trim(),
          mode: _mode,
          endpoint: _endpoint.text.trim(),
          secrets: secrets.isEmpty ? null : secrets,
        );
      }
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(true);
    } on Object catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _saving = false;
        _error = '$e';
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final isLinkup = _kind == 'linkup';
    return AlertDialog(
      title: Text(
        widget.existing == null ? 'Add integration' : 'Edit integration',
      ),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              key: const Key('tool-integration-name'),
              controller: _name,
              decoration: const InputDecoration(
                labelText: 'Name',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 12),
            if (widget.existing == null)
              DropdownButtonFormField<String>(
                key: const Key('tool-integration-kind'),
                initialValue: _kind,
                decoration: const InputDecoration(
                  labelText: 'Kind',
                  border: OutlineInputBorder(),
                ),
                items: const [
                  DropdownMenuItem(value: 'searxng', child: Text('SearXNG')),
                  DropdownMenuItem(value: 'linkup', child: Text('Linkup')),
                  DropdownMenuItem(value: 'get_md', child: Text('get-md')),
                  DropdownMenuItem(value: 'crawl4ai', child: Text('Crawl4AI')),
                ],
                onChanged: (v) {
                  if (v == null) {
                    return;
                  }
                  setState(() {
                    _kind = v;
                    if (v == 'linkup') {
                      _mode = 'external';
                    }
                  });
                },
              ),
            const SizedBox(height: 12),
            DropdownButtonFormField<String>(
              key: const Key('tool-integration-mode'),
              initialValue: _mode,
              decoration: const InputDecoration(
                labelText: 'Mode',
                border: OutlineInputBorder(),
              ),
              items: [
                if (!isLinkup)
                  const DropdownMenuItem(
                    value: 'bundled',
                    child: Text('Bundled'),
                  ),
                const DropdownMenuItem(
                  value: 'external',
                  child: Text('External'),
                ),
              ],
              onChanged: (v) {
                if (v != null) {
                  setState(() => _mode = v);
                }
              },
            ),
            const SizedBox(height: 12),
            TextField(
              key: const Key('tool-integration-endpoint'),
              controller: _endpoint,
              decoration: InputDecoration(
                labelText: 'Endpoint',
                border: const OutlineInputBorder(),
                helperText: _mode == 'bundled'
                    ? 'Leave blank to use the bundled DNS endpoint'
                    : 'https://…',
              ),
            ),
            if (isLinkup) ...[
              const SizedBox(height: 12),
              TextField(
                key: const Key('tool-integration-api-key'),
                controller: _apiKey,
                obscureText: true,
                decoration: InputDecoration(
                  labelText: 'API key',
                  border: const OutlineInputBorder(),
                  helperText: widget.existing?.apiKeyConfigured == true
                      ? 'Configured — leave blank to keep'
                      : null,
                ),
              ),
            ],
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _saving ? null : () => Navigator.pop(context, false),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const Key('tool-integration-save'),
          onPressed: _saving ? null : () => unawaited(_submit()),
          child: const Text('Save'),
        ),
      ],
    );
  }
}
