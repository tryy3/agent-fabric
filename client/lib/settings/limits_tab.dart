import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../ui/theme/design_tokens.dart';
import 'limits_fields.dart';

/// Settings tab for the plane-wide run budget: tool rounds and wall-clock time.
/// Assistants may lower or raise these, never above the plane maximum.
class LimitsTab extends StatefulWidget {
  const LimitsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<LimitsTab> createState() => _LimitsTabState();
}

class _LimitsTabState extends State<LimitsTab> {
  LimitsDraft? _draft;
  OperatorFailure? _failure;
  bool _saving = false;
  bool _dirty = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  @override
  void dispose() {
    _draft?.dispose();
    super.dispose();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('limits load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() => _failure = null);
    try {
      final settings = await widget.catalog.getSettings();
      if (!mounted) return;
      final previous = _draft;
      setState(() {
        _draft = LimitsDraft(json: settings.limits);
        _dirty = false;
        _error = null;
      });
      previous?.dispose();
    } on Object catch (e, s) {
      AppLog.record('limits load failed: $e', s);
      if (!mounted) return;
      setState(() => _failure = operatorFailureFrom(e));
    }
  }

  Future<void> _save() async {
    final draft = _draft;
    if (draft == null || draft.error != null) return;
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.catalog.patchSettings(limits: draft.toPlaneJson());
      if (!mounted) return;
      setState(() {
        _saving = false;
        _dirty = false;
      });
    } on Object catch (e, s) {
      AppLog.record('limits save failed: $e', s);
      if (!mounted) return;
      setState(() {
        _saving = false;
        _error = operatorMessageFor(operatorFailureFrom(e));
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final draft = _draft;
    if (_failure case final failure?) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(operatorMessageFor(failure), textAlign: TextAlign.center),
              const SizedBox(height: 16),
              Semantics(
                button: true,
                label: 'Retry',
                child: FilledButton(
                  onPressed: _startLoad,
                  child: const Text('Retry'),
                ),
              ),
            ],
          ),
        ),
      );
    }
    if (draft == null) {
      return const Center(child: CircularProgressIndicator());
    }
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        Text('Run limits', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        LimitsFields(
          draft: draft,
          inheritFrom: 'the built-in value',
          onChanged: () => setState(() {
            _dirty = true;
          }),
        ),
        const SizedBox(height: 16),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(_error!, style: TextStyle(color: tokens.error)),
          ),
        Align(
          alignment: Alignment.centerLeft,
          child: FilledButton(
            key: const Key('limits-save'),
            onPressed: _dirty && !_saving && draft.error == null
                ? () => unawaited(_save())
                : null,
            child: Text(_saving ? 'Saving…' : 'Save'),
          ),
        ),
      ],
    );
  }
}
