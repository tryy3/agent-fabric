import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';

/// Model specs: the synced models.dev-shaped database. Shows when it was last
/// synced, lets the operator change the source and sync now, and browses
/// providers with model capabilities.
class ModelSpecsTab extends StatefulWidget {
  const ModelSpecsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<ModelSpecsTab> createState() => _ModelSpecsTabState();
}

class _ModelSpecsTabState extends State<ModelSpecsTab> {
  ModelSpecsStatus? _status;
  String? _error;
  bool _loading = true;
  bool _syncing = false;
  final _source = TextEditingController();

  @override
  void initState() {
    super.initState();
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('model specs load: $e', s);
      }),
    );
  }

  @override
  void dispose() {
    _source.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final status = await widget.catalog.getModelSpecsStatus();
      if (!mounted) return;
      setState(() {
        _status = status;
        _source.text = status.sourceUrl;
        _loading = false;
        _error = null;
      });
    } on Object catch (e) {
      if (!mounted) return;
      setState(() {
        _error = operatorMessageFromError(e);
        _loading = false;
      });
    }
  }

  void _onSync() {
    unawaited(
      _sync().catchError((Object e, StackTrace s) {
        AppLog.record('model specs sync: $e', s);
      }),
    );
  }

  Future<void> _sync() async {
    setState(() {
      _syncing = true;
      _error = null;
    });
    try {
      // Save the source first so "sync now" uses what the field shows.
      await widget.catalog.updateModelSpecsSettings(
        sourceUrl: _source.text.trim(),
      );
      await widget.catalog.syncModelSpecs();
    } on Object catch (e) {
      if (mounted) setState(() => _error = operatorMessageFromError(e));
    }
    await _load();
    if (mounted) setState(() => _syncing = false);
  }

  void _onToggleEnabled(bool value) {
    unawaited(
      widget.catalog
          .updateModelSpecsSettings(enabled: value)
          .then((_) => _load())
          .catchError((Object e, StackTrace s) {
            AppLog.record('model specs enable: $e', s);
          }),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    final tokens = designTokensOf(context);
    final status = _status;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            FilledButton(
              key: const Key('model-specs-sync'),
              onPressed: _syncing ? null : _onSync,
              child: Text(_syncing ? 'Syncing…' : 'Sync now'),
            ),
            const SizedBox(width: 12),
            if (status != null)
              Expanded(
                child: Text(
                  _syncSummary(status),
                  key: const Key('model-specs-summary'),
                  style: TextStyle(fontSize: 12, color: tokens.textMuted),
                ),
              ),
          ],
        ),
        if (status != null)
          SwitchListTile(
            key: const Key('model-specs-auto'),
            contentPadding: EdgeInsets.zero,
            title: Text(
              'Sync automatically every ${status.syncIntervalHours} hours',
            ),
            value: status.enabled,
            onChanged: _onToggleEnabled,
          ),
        ExpansionTile(
          key: const Key('model-specs-source-section'),
          tilePadding: EdgeInsets.zero,
          maintainState: true,
          title: const Text('Source'),
          subtitle: Text(
            _source.text.trim().isEmpty
                ? (status?.effectiveSourceUrl ?? 'models.dev')
                : _source.text.trim(),
            style: TextStyle(fontSize: 12, color: tokens.textMuted),
          ),
          children: [
            TextField(
              key: const Key('model-specs-source'),
              controller: _source,
              decoration: InputDecoration(
                labelText: 'Source URL',
                hintText:
                    status?.effectiveSourceUrl ?? 'https://models.dev/api.json',
                helperText:
                    'Empty uses models.dev. Any URL serving the same api.json '
                    'structure works. Applied on the next sync.',
              ),
            ),
          ],
        ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(_error!, style: TextStyle(color: tokens.error)),
          ),
        if (status != null && status.lastError.isNotEmpty && _error == null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              'Last attempt failed: ${status.lastError}',
              style: TextStyle(color: tokens.warning),
            ),
          ),
      ],
    );
  }

  String _syncSummary(ModelSpecsStatus s) {
    final at = s.lastSyncedAt;
    final when = at == null ? 'never synced' : 'last synced ${_ago(at)}';
    return '$when · ${s.providerCount} providers · ${s.modelCount} models';
  }

  String _ago(DateTime t) {
    final d = DateTime.now().difference(t);
    if (d.inMinutes < 1) return 'just now';
    if (d.inHours < 1) return '${d.inMinutes} min ago';
    if (d.inDays < 1) return '${d.inHours} h ago';
    return '${d.inDays} d ago';
  }
}
