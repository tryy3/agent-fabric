import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:agent_fabric_client/settings/instruction_variables.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import 'gate_instructions_editor.dart';

/// Load state for plane instruction settings.
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
  const _InstructionsReady({
    required this.platformInstructions,
    required this.runtimeContext,
  });

  final String platformInstructions;
  final String runtimeContext;
}

/// The sections of the Instructions settings tab.
enum _InstructionsSection {
  effective('Effective instructions'),
  gate('Gate instructions');

  _InstructionsSection(this.label);

  final String label;
}

/// Settings tab hosting the Effective instructions and Gate instructions
/// sections. Gate instructions are never part of the effective instructions,
/// so they stay a separate section with its own load and Save.
class InstructionsTab extends StatefulWidget {
  const InstructionsTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<InstructionsTab> createState() => _InstructionsTabState();
}

class _InstructionsTabState extends State<InstructionsTab> {
  _InstructionsSection _section = _InstructionsSection.effective;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(24, 16, 24, 0),
            child: Align(
              alignment: Alignment.centerLeft,
              child: Semantics(
                container: true,
                label: 'Instructions section',
                child: SegmentedButton<_InstructionsSection>(
                  key: const Key('instructions-section'),
                  segments: [
                    for (final s in _InstructionsSection.values)
                      ButtonSegment(value: s, label: Text(s.label)),
                  ],
                  selected: {_section},
                  onSelectionChanged: (next) =>
                      setState(() => _section = next.single),
                ),
              ),
            ),
          ),
          Expanded(
            child: IndexedStack(
              index: _section.index,
              children: [
                _EffectiveInstructionsSection(catalog: widget.catalog),
                GateInstructionsSection(catalog: widget.catalog),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _EffectiveInstructionsSection extends StatefulWidget {
  const _EffectiveInstructionsSection({required this.catalog});

  final CatalogClient catalog;

  @override
  State<_EffectiveInstructionsSection> createState() =>
      _EffectiveInstructionsSectionState();
}

class _EffectiveInstructionsSectionState
    extends State<_EffectiveInstructionsSection> {
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
          platformInstructions: settings.platformInstructions,
          runtimeContext: settings.runtimeContext,
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
    return switch (_state) {
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
      _InstructionsReady(:final platformInstructions, :final runtimeContext) =>
        _PlaneInstructionsEditor(
          key: ValueKey('$platformInstructions|$runtimeContext'),
          initialPlatform: platformInstructions,
          initialRuntime: runtimeContext,
          onSave: (platform, runtime) async {
            final updated = await widget.catalog.patchSettings(
              platformInstructions: platform,
              runtimeContext: runtime,
            );
            if (!mounted) {
              return;
            }
            setState(() {
              _state = _InstructionsReady(
                platformInstructions: updated.platformInstructions,
                runtimeContext: updated.runtimeContext,
              );
            });
          },
        ),
    };
  }
}

class _PlaneInstructionsEditor extends StatefulWidget {
  const _PlaneInstructionsEditor({
    super.key,
    required this.initialPlatform,
    required this.initialRuntime,
    required this.onSave,
  });

  final String initialPlatform;
  final String initialRuntime;
  final Future<void> Function(String platform, String runtime) onSave;

  @override
  State<_PlaneInstructionsEditor> createState() =>
      _PlaneInstructionsEditorState();
}

class _PlaneInstructionsEditorState extends State<_PlaneInstructionsEditor> {
  late final TextEditingController _platform;
  late final TextEditingController _runtime;
  late final FocusNode _platformFocus;
  late final FocusNode _runtimeFocus;
  String _selectedToken = kInstructionVariables.first.token;
  String? _error;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _platform = TextEditingController(text: widget.initialPlatform);
    _runtime = TextEditingController(text: widget.initialRuntime);
    _platformFocus = FocusNode();
    _runtimeFocus = FocusNode();
    _platformFocus.addListener(_trackPlatformFocus);
    _runtimeFocus.addListener(_trackRuntimeFocus);
  }

  @override
  void dispose() {
    _platform.dispose();
    _runtime.dispose();
    _platformFocus.dispose();
    _runtimeFocus.dispose();
    super.dispose();
  }

  // The Insert button takes focus on click, so remember the last-focused field
  // rather than reading hasFocus at click time. Defaults to runtime context,
  // where variables usually live.
  bool _platformWasLast = false;

  void _trackPlatformFocus() {
    if (_platformFocus.hasFocus) {
      _platformWasLast = true;
    }
  }

  void _trackRuntimeFocus() {
    if (_runtimeFocus.hasFocus) {
      _platformWasLast = false;
    }
  }

  TextEditingController get _insertTarget =>
      _platformWasLast ? _platform : _runtime;

  FocusNode get _insertFocus =>
      _platformWasLast ? _platformFocus : _runtimeFocus;

  void _insertSelected() {
    final controller = _insertTarget;
    final focus = _insertFocus;
    final token = _selectedToken;
    final value = controller.value;
    final selection = value.selection;
    final start = selection.isValid ? selection.start : value.text.length;
    final end = selection.isValid ? selection.end : value.text.length;
    final next = value.text.replaceRange(start, end, token);
    controller.value = TextEditingValue(
      text: next,
      selection: TextSelection.collapsed(offset: start + token.length),
    );
    focus.requestFocus();
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.onSave(_platform.text, _runtime.text);
      if (!mounted) {
        return;
      }
      setState(() => _saving = false);
    } on Object catch (e, s) {
      AppLog.record('plane instructions save failed: $e', s);
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
    final theme = Theme.of(context);
    final rail = InstructionVariablesRail(
      key: const Key('instruction-variables-rail'),
      selectedToken: _selectedToken,
      onSelect: (token) => setState(() => _selectedToken = token),
      onInsert: _insertSelected,
    );

    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Write platform policy and runtime facts. Use the Variables rail '
            'to understand and insert placeholders. Values are pinned when a '
            'thread session starts.',
            style: theme.textTheme.bodySmall,
          ),
          const SizedBox(height: 20),
          Expanded(
            child: LayoutBuilder(
              builder: (context, constraints) {
                final editors = _EditorsColumn(
                  platform: _platform,
                  runtime: _runtime,
                  platformFocus: _platformFocus,
                  runtimeFocus: _runtimeFocus,
                );
                // Side-by-side editors + rail when width allows; otherwise
                // stack editors above a scrollable rail.
                if (constraints.maxWidth >= 900) {
                  return Row(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Expanded(flex: 3, child: editors),
                      const SizedBox(width: 16),
                      SizedBox(
                        width: 280,
                        child: SingleChildScrollView(child: rail),
                      ),
                    ],
                  );
                }
                return Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Expanded(flex: 3, child: editors),
                    const SizedBox(height: 16),
                    Expanded(
                      flex: 2,
                      child: SingleChildScrollView(child: rail),
                    ),
                  ],
                );
              },
            ),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: TextStyle(color: theme.colorScheme.error)),
          ],
          const SizedBox(height: 16),
          Align(
            alignment: Alignment.centerRight,
            child: Semantics(
              button: true,
              label: 'Save instructions',
              child: FilledButton(
                key: const Key('instructions-save'),
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

class _EditorsColumn extends StatelessWidget {
  const _EditorsColumn({
    required this.platform,
    required this.runtime,
    required this.platformFocus,
    required this.runtimeFocus,
  });

  final TextEditingController platform;
  final TextEditingController runtime;
  final FocusNode platformFocus;
  final FocusNode runtimeFocus;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: _InstructionField(
            key: const Key('platform-instructions'),
            controller: platform,
            focusNode: platformFocus,
            title: 'Platform instructions',
            description:
                'Shared guidance for how assistants use tools, work '
                'in the environment, and handle tasks. Applies to '
                'every assistant.',
            hintText: 'Optional platform instructions…',
          ),
        ),
        const SizedBox(height: 16),
        Expanded(
          child: _InstructionField(
            key: const Key('runtime-context'),
            controller: runtime,
            focusNode: runtimeFocus,
            title: 'Runtime context',
            description:
                'Session-specific facts supplied by the platform, '
                'such as date, timezone, model and workspace.',
            hintText:
                'Example: Current date: {{currentDate}}\n'
                'Timezone: {{timezone}}\n'
                'Workspace: {{workspaceRoot}}\n'
                'Model: {{modelId}}',
          ),
        ),
      ],
    );
  }
}

class _InstructionField extends StatelessWidget {
  const _InstructionField({
    super.key,
    required this.controller,
    required this.focusNode,
    required this.title,
    required this.description,
    required this.hintText,
  });

  final TextEditingController controller;
  final FocusNode focusNode;
  final String title;
  final String description;
  final String hintText;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(title, style: theme.textTheme.titleSmall),
        const SizedBox(height: 8),
        Text(description, style: theme.textTheme.bodySmall),
        const SizedBox(height: 12),
        Expanded(
          child: TextField(
            controller: controller,
            focusNode: focusNode,
            maxLines: null,
            expands: true,
            textAlignVertical: TextAlignVertical.top,
            decoration: InputDecoration(
              border: const OutlineInputBorder(),
              alignLabelWithHint: true,
              hintText: hintText,
            ),
          ),
        ),
      ],
    );
  }
}
