import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';

/// Built-in budget limits, mirroring catalog.DefaultToolRounds,
/// catalog.DefaultMaxToolRounds and catalog.DefaultMaxTurnSeconds on the plane.
const int builtInDefaultToolRounds = 8;
const int builtInMaxToolRounds = 100;
const int builtInMaxTurnSeconds = 900;

const int _maxRounds = 1000;
const int _minSeconds = 10;
const int _maxSeconds = 86400;

/// The three budget limits as editable text. An empty field means unset: the
/// plane falls back to the next level (assistant to plane, plane to built-in).
class LimitsDraft {
  LimitsDraft({Object? json})
    : defaultToolRounds = TextEditingController(
        text: _text(json, 'defaultToolRounds'),
      ),
      maxToolRounds = TextEditingController(text: _text(json, 'maxToolRounds')),
      maxTurnSeconds = TextEditingController(
        text: _text(json, 'maxTurnSeconds'),
      );

  final TextEditingController defaultToolRounds;
  final TextEditingController maxToolRounds;
  final TextEditingController maxTurnSeconds;

  static String _text(Object? json, String key) {
    if (json is! Map) return '';
    final v = json[key];
    return v is num ? '${v.toInt()}' : '';
  }

  void dispose() {
    defaultToolRounds.dispose();
    maxToolRounds.dispose();
    maxTurnSeconds.dispose();
  }

  static int? _parse(TextEditingController c) => int.tryParse(c.text.trim());

  static String? _check(
    TextEditingController c,
    int min,
    int max,
    String label,
  ) {
    final raw = c.text.trim();
    if (raw.isEmpty) return null;
    final v = int.tryParse(raw);
    if (v == null || v < min || v > max) {
      return '$label must be a whole number from $min to $max';
    }
    return null;
  }

  String? get defaultRoundsError =>
      _check(defaultToolRounds, 1, _maxRounds, 'Default tool rounds');

  String? get maxRoundsError =>
      _check(maxToolRounds, 1, _maxRounds, 'Max tool rounds');

  String? get secondsError =>
      _check(maxTurnSeconds, _minSeconds, _maxSeconds, 'Max turn time');

  /// Why the draft cannot be saved, or null when it can.
  String? get error {
    final own = defaultRoundsError ?? maxRoundsError ?? secondsError;
    if (own != null) return own;
    final def = _parse(defaultToolRounds);
    final max = _parse(maxToolRounds);
    if (def != null && max != null && def > max) {
      return 'Default tool rounds may not exceed max tool rounds';
    }
    return null;
  }

  /// The limits object for a PATCH: set fields only. Plane settings replace the
  /// whole object, so this is complete.
  Map<String, dynamic> toPlaneJson() => {
    if (_parse(defaultToolRounds) case final v?) 'defaultToolRounds': v,
    if (_parse(maxToolRounds) case final v?) 'maxToolRounds': v,
    if (_parse(maxTurnSeconds) case final v?) 'maxTurnSeconds': v,
  };

  /// The limits object for an assistant settings PATCH, which merges: an empty
  /// field is sent as null so it clears the stored value.
  Map<String, dynamic> toMergePatch() => {
    'defaultToolRounds': _parse(defaultToolRounds),
    'maxToolRounds': _parse(maxToolRounds),
    'maxTurnSeconds': _parse(maxTurnSeconds),
  };
}

/// Three number fields for the budget limits of a plane or an assistant.
class LimitsFields extends StatelessWidget {
  const LimitsFields({
    super.key,
    required this.draft,
    required this.onChanged,
    this.inheritFrom = 'the plane setting',
    this.defaultRounds = builtInDefaultToolRounds,
    this.maxRounds = builtInMaxToolRounds,
    this.maxSeconds = builtInMaxTurnSeconds,
  });

  final LimitsDraft draft;
  final VoidCallback onChanged;

  /// Where an empty field takes its value from, for the helper text.
  final String inheritFrom;

  /// The value an empty field resolves to, shown as the hint.
  final int defaultRounds;
  final int maxRounds;
  final int maxSeconds;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          'A tool round is one step where the model asks for tools and gets '
          'the results. When a run uses its rounds or its time, the model '
          'gets one last turn without tools to write its best answer. '
          'Empty fields use $inheritFrom.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 12,
          runSpacing: 12,
          children: [
            _field(
              key: const Key('limits-default-rounds'),
              controller: draft.defaultToolRounds,
              label: 'Default tool rounds',
              hint: '$defaultRounds',
              helper: 'For a normal message',
              error: draft.defaultRoundsError,
            ),
            _field(
              key: const Key('limits-max-rounds'),
              controller: draft.maxToolRounds,
              label: 'Max tool rounds',
              hint: '$maxRounds',
              helper: 'Ceiling for any run, skills included',
              error: draft.maxRoundsError,
            ),
            _field(
              key: const Key('limits-max-seconds'),
              controller: draft.maxTurnSeconds,
              label: 'Max turn time (seconds)',
              hint: '$maxSeconds',
              helper: 'Wall-clock limit of one run',
              error: draft.secondsError,
            ),
          ],
        ),
        if (draft.error case final message?
            when draft.defaultRoundsError == null &&
                draft.maxRoundsError == null &&
                draft.secondsError == null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              message,
              key: const Key('limits-order-error'),
              style: TextStyle(color: tokens.error, fontSize: 12),
            ),
          ),
      ],
    );
  }

  Widget _field({
    required Key key,
    required TextEditingController controller,
    required String label,
    required String hint,
    required String helper,
    required String? error,
  }) {
    return SizedBox(
      width: 220,
      child: TextField(
        key: key,
        controller: controller,
        keyboardType: TextInputType.number,
        decoration: InputDecoration(
          labelText: label,
          hintText: hint,
          helperText: helper,
          errorText: error,
          border: const OutlineInputBorder(),
          isDense: true,
        ),
        onChanged: (_) => onChanged(),
      ),
    );
  }
}
