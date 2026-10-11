import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../ui/theme/design_tokens.dart';

/// Longest gate instructions text the plane stores.
const gateInstructionsMaxChars = 20000;

/// How much of the text a gate scorer reads when
/// `permissions.scorers.instructionsMaxChars` is unset.
const gateScorerDefaultChars = 4000;

/// What gate instructions are, shown under every gate instructions editor.
const gateInstructionsHelp =
    'Your own rules for what the agent may and may not do. Only you write '
    'them. The gate scorers read them and the agent can read them, but they '
    'are never part of its effective instructions. They are pinned when a '
    'session starts, so changes apply from the next session.';

const gateInstructionsHint =
    'MUST NOT:\n'
    '- Push to main\n'
    '- Delete files outside the project\n'
    'MAY:\n'
    '- Push to feature branches';

/// The scorer text cap from stored `permissions.scorers`, or the default.
int gateScorerCapFrom(Object? permissions) {
  final scorers = permissions is Map ? permissions['scorers'] : null;
  final raw = scorers is Map ? scorers['instructionsMaxChars'] : null;
  return raw is int && raw > 0 ? raw : gateScorerDefaultChars;
}

/// A multiline gate instructions field with a character counter that warns
/// when the text is over the plane limit or past what the scorers read.
class GateInstructionsField extends StatelessWidget {
  const GateInstructionsField({
    super.key,
    required this.controller,
    required this.scorerCap,
    this.enabled = true,
    this.expands = false,
    this.onChanged,
  });

  final TextEditingController controller;
  final int scorerCap;
  final bool enabled;

  /// Fill the parent's height instead of showing a few lines.
  final bool expands;
  final VoidCallback? onChanged;

  @override
  Widget build(BuildContext context) {
    final field = TextField(
      key: const Key('gate-instructions-field'),
      controller: controller,
      enabled: enabled,
      minLines: expands ? null : 4,
      maxLines: expands ? null : 12,
      expands: expands,
      textAlignVertical: TextAlignVertical.top,
      decoration: const InputDecoration(
        border: OutlineInputBorder(),
        alignLabelWithHint: true,
        hintText: gateInstructionsHint,
      ),
      onChanged: onChanged == null ? null : (_) => onChanged!(),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (expands) Expanded(child: field) else field,
        const SizedBox(height: 6),
        ListenableBuilder(
          listenable: controller,
          builder: (context, _) => GateInstructionsCounter(
            length: controller.text.runes.length,
            scorerCap: scorerCap,
          ),
        ),
      ],
    );
  }
}

/// `1234 / 20000` with a warning once the scorers stop reading the text.
class GateInstructionsCounter extends StatelessWidget {
  const GateInstructionsCounter({
    super.key,
    required this.length,
    required this.scorerCap,
  });

  final int length;
  final int scorerCap;

  /// Whether the text is too long to save.
  static bool tooLong(int length) => length > gateInstructionsMaxChars;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final over = tooLong(length);
    final warn = !over && length > scorerCap;
    final style = TextStyle(
      fontSize: 12,
      color: over
          ? tokens.error
          : warn
          ? tokens.warning
          : tokens.textMuted,
    );
    final message = over
        ? 'Too long: at most $gateInstructionsMaxChars characters can be '
              'saved.'
        : warn
        ? 'The scorers read only the first $scorerCap characters. Text after '
              'that is not applied by them.'
        : null;
    return Column(
      key: const Key('gate-instructions-counter'),
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('$length / $gateInstructionsMaxChars', style: style),
        if (message != null)
          Text(
            message,
            key: const Key('gate-instructions-warning'),
            style: style,
          ),
      ],
    );
  }
}

/// How an assistant relates to the plane's gate instructions.
enum GateInstructionsMode {
  /// Use the plane text (stored as null).
  inherit('Inherit plane text', 'Uses the plane-wide gate instructions.'),

  /// No rules for this assistant (stored as an empty string).
  none('No rules', 'This assistant has no gate instructions at all.'),

  /// The assistant's own text replaces the plane text.
  custom('Custom text', 'Replaces the plane text for this assistant.');

  GateInstructionsMode(this.label, this.help);

  final String label;
  final String help;

  static GateInstructionsMode of(String? stored) => switch (stored) {
    null => inherit,
    '' => none,
    _ => custom,
  };

  /// The PATCH value: null inherits, "" clears, text replaces.
  String? valueFor(String text) => switch (this) {
    inherit => null,
    none => '',
    custom => text,
  };

  /// Whether [text] can be saved in this mode.
  bool isValid(String text) =>
      this != custom ||
      (text.isNotEmpty && !GateInstructionsCounter.tooLong(text.runes.length));
}

/// The assistant editor's gate instructions override: a mode picker and, for
/// custom text, the editor.
class GateInstructionsOverride extends StatelessWidget {
  const GateInstructionsOverride({
    super.key,
    required this.mode,
    required this.controller,
    required this.scorerCap,
    required this.onModeChanged,
    required this.onChanged,
  });

  final GateInstructionsMode mode;
  final TextEditingController controller;
  final int scorerCap;
  final ValueChanged<GateInstructionsMode> onModeChanged;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final helpStyle = TextStyle(color: tokens.textSecondary, fontSize: 12);
    return Padding(
      padding: const EdgeInsets.only(top: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          DropdownButtonFormField<GateInstructionsMode>(
            key: const Key('assistant-gate-instructions-mode'),
            initialValue: mode,
            decoration: const InputDecoration(
              labelText: 'Gate instructions',
              border: OutlineInputBorder(),
              isDense: true,
            ),
            items: [
              for (final m in GateInstructionsMode.values)
                DropdownMenuItem(value: m, child: Text(m.label)),
            ],
            onChanged: (m) {
              if (m != null) onModeChanged(m);
            },
          ),
          const SizedBox(height: 6),
          Text(mode.help, style: helpStyle),
          Text(gateInstructionsHelp, style: helpStyle),
          if (mode == GateInstructionsMode.custom) ...[
            const SizedBox(height: 8),
            GateInstructionsField(
              controller: controller,
              scorerCap: scorerCap,
              onChanged: onChanged,
            ),
          ],
        ],
      ),
    );
  }
}

/// Plane-wide Gate instructions section of the Instructions settings tab.
class GateInstructionsSection extends StatefulWidget {
  const GateInstructionsSection({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<GateInstructionsSection> createState() =>
      _GateInstructionsSectionState();
}

class _GateInstructionsSectionState extends State<GateInstructionsSection> {
  final _text = TextEditingController();
  int _scorerCap = gateScorerDefaultChars;
  bool _loading = true;
  bool _saving = false;
  bool _saved = false;
  OperatorFailure? _loadFailure;
  String? _error;

  @override
  void initState() {
    super.initState();
    _startLoad();
  }

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  void _startLoad() {
    unawaited(
      _load().catchError((Object e, StackTrace s) {
        AppLog.record('gate instructions load: $e', s);
      }),
    );
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loadFailure = null;
    });
    try {
      final settings = await widget.catalog.getSettings();
      if (!mounted) {
        return;
      }
      setState(() {
        _text.text = settings.gateInstructions;
        _scorerCap = gateScorerCapFrom(settings.permissions);
        _loading = false;
      });
    } on Object catch (e, s) {
      AppLog.record('gate instructions load failed: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _loadFailure = operatorFailureFrom(e);
        _loading = false;
      });
    }
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _saved = false;
      _error = null;
    });
    try {
      final updated = await widget.catalog.patchSettings(
        gateInstructions: _text.text,
      );
      if (!mounted) {
        return;
      }
      setState(() {
        _scorerCap = gateScorerCapFrom(updated.permissions);
        _saving = false;
        _saved = true;
      });
    } on Object catch (e, s) {
      AppLog.record('gate instructions save failed: $e', s);
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
    final failure = _loadFailure;
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (failure != null) {
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
    final tooLong = GateInstructionsCounter.tooLong(_text.text.runes.length);
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(gateInstructionsHelp, style: theme.textTheme.bodySmall),
          const SizedBox(height: 4),
          Text(
            'This is the plane-wide default. An assistant can inherit it, '
            'replace it or turn it off in its own settings.',
            style: theme.textTheme.bodySmall,
          ),
          const SizedBox(height: 16),
          Expanded(
            child: GateInstructionsField(
              controller: _text,
              scorerCap: _scorerCap,
              expands: true,
              onChanged: () => setState(() => _saved = false),
            ),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: TextStyle(color: theme.colorScheme.error)),
          ],
          const SizedBox(height: 16),
          Align(
            alignment: Alignment.centerRight,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (_saved)
                  Padding(
                    padding: const EdgeInsets.only(right: 12),
                    child: Text(
                      'Saved. Applies from the next session.',
                      key: const Key('gate-instructions-saved'),
                      style: theme.textTheme.bodySmall,
                    ),
                  ),
                Semantics(
                  button: true,
                  label: 'Save gate instructions',
                  child: FilledButton(
                    key: const Key('gate-instructions-save'),
                    onPressed: _saving || tooLong
                        ? null
                        : () => unawaited(_save()),
                    child: Text(_saving ? 'Saving…' : 'Save'),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
