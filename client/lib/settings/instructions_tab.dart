import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';

/// Load state for Harness instructions settings.
sealed class _InstructionsLoadState {
  const _InstructionsLoadState();
}

final class _InstructionsLoading extends _InstructionsLoadState {
  const _InstructionsLoading();
}

final class _InstructionsFailed extends _InstructionsLoadState {
  const _InstructionsFailed(this.failure);

  final OperatorFailure failure;
}

final class _InstructionsReady extends _InstructionsLoadState {
  const _InstructionsReady({required this.harnessInstructions});

  final String harnessInstructions;
}

/// Settings tab for plane-wide Harness instructions.
class InstructionsTab extends StatefulWidget {
  const InstructionsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<InstructionsTab> createState() => _InstructionsTabState();
}

class _InstructionsTabState extends State<InstructionsTab> {
  _InstructionsLoadState _state = const _InstructionsLoading();

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('instructions load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() => _state = const _InstructionsLoading());
    try {
      final settings = await widget.catalog.getSettings();
      if (!mounted) {
        return;
      }
      setState(() {
        _state = _InstructionsReady(
          harnessInstructions: settings.harnessInstructions,
        );
      });
    } on Object catch (e, s) {
      AppLog.record('instructions load failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _state = _InstructionsFailed(operatorFailureFrom(e)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: switch (_state) {
        _InstructionsLoading() => const Center(
          child: CircularProgressIndicator(),
        ),
        _InstructionsFailed(:final failure) => Center(
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
        ),
        _InstructionsReady(:final harnessInstructions) =>
          _HarnessInstructionsEditor(
            key: ValueKey(harnessInstructions),
            initial: harnessInstructions,
            onSave: (next) async {
              final updated = await widget.catalog.patchSettings(
                harnessInstructions: next,
              );
              if (!mounted) {
                return;
              }
              setState(() {
                _state = _InstructionsReady(
                  harnessInstructions: updated.harnessInstructions,
                );
              });
            },
          ),
      },
    );
  }
}

class _HarnessInstructionsEditor extends StatefulWidget {
  const _HarnessInstructionsEditor({
    super.key,
    required this.initial,
    required this.onSave,
  });

  final String initial;
  final Future<void> Function(String next) onSave;

  @override
  State<_HarnessInstructionsEditor> createState() =>
      _HarnessInstructionsEditorState();
}

class _HarnessInstructionsEditorState
    extends State<_HarnessInstructionsEditor> {
  late final TextEditingController _controller;
  String? _error;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController(text: widget.initial);
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.onSave(_controller.text);
      if (!mounted) {
        return;
      }
      setState(() => _saving = false);
    } on Object catch (e, s) {
      AppLog.record('harness instructions save failed: $e', s);
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
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Harness instructions',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 8),
          Text(
            'Plane-wide defaults for runtime methodology, tool use, and shared '
            'behavior. Composed with Assistant instructions at session start. '
            'Empty is omitted.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 16),
          Expanded(
            child: TextField(
              key: const Key('harness-instructions'),
              controller: _controller,
              maxLines: null,
              expands: true,
              textAlignVertical: TextAlignVertical.top,
              decoration: const InputDecoration(
                border: OutlineInputBorder(),
                alignLabelWithHint: true,
                hintText: 'Optional Harness instructions…',
              ),
            ),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ],
          const SizedBox(height: 16),
          Align(
            alignment: Alignment.centerRight,
            child: Semantics(
              button: true,
              label: 'Save Harness instructions',
              child: FilledButton(
                key: const Key('harness-instructions-save'),
                onPressed: _saving ? null : () => unawaited(_save()),
                child: Text(_saving ? 'Saving…' : 'Save'),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
