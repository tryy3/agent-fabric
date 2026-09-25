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
  const _IntegrationsReady({required this.apiKey, required this.accountId});

  final String apiKey;
  final String accountId;
}

/// Plane-level third-party publish credentials.
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
      setState(() {
        _state = _IntegrationsReady(apiKey: apiKey, accountId: accountId);
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

  Future<void> _save() async {
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
          .showSnackBar(const SnackBar(content: Text('Integrations saved')));
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
      _IntegrationsReady() => ListView(
        key: const Key('integrations-tab'),
        padding: const EdgeInsets.all(24),
        children: [
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
                        _save().catchError((Object e, StackTrace s) {
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
                  : const Text('Save'),
            ),
          ),
        ],
      ),
    };
  }
}
