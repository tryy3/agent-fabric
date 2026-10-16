import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';
import 'permissions_editor.dart' show permissionBandLabel;

/// The built-in thresholds of one permission mode (decision 22). A null
/// [askAt] never asks.
typedef PermissionPolicyDefault = ({
  String mode,
  String name,
  int? askAt,
  int cancelAt,
});

/// The built-in thresholds of every mode, least to most permissive. Mirrors
/// gate.DefaultPolicies on the plane.
const permissionPolicyDefaults = <PermissionPolicyDefault>[
  (mode: 'ask', name: 'Ask for approval', askAt: 3, cancelAt: 9),
  (mode: 'auto_approve', name: 'Approve for me', askAt: 5, cancelAt: 9),
  (mode: 'auto', name: 'Run automatically', askAt: 7, cancelAt: 9),
  (mode: 'full', name: 'Full access', askAt: null, cancelAt: 10),
];

/// The editable thresholds of one mode: the built-in values unless changed.
class PermissionPolicyDraft {
  PermissionPolicyDraft(this.defaults, {int? askAt, int? cancelAt})
    : askAt = askAt ?? defaults.askAt,
      cancelAt = cancelAt ?? defaults.cancelAt;

  final PermissionPolicyDefault defaults;

  /// The score asking starts at; null for Full access, which never asks.
  int? askAt;
  int cancelAt;

  bool get isModified =>
      askAt != defaults.askAt || cancelAt != defaults.cancelAt;

  /// Whether the thresholds are consistent: asking cannot start above
  /// cancelling.
  bool get isValid => askAt == null || askAt! <= cancelAt;

  /// Whether some score makes this mode ask.
  bool get asks => askAt != null && askAt! < cancelAt;

  void reset() {
    askAt = defaults.askAt;
    cancelAt = defaults.cancelAt;
  }

  /// The override sent to the plane, or null when nothing changed.
  Map<String, Object?>? toJson() {
    if (!isModified) return null;
    return {
      if (askAt != defaults.askAt) 'askAt': askAt,
      if (cancelAt != defaults.cancelAt) 'cancelAt': cancelAt,
    };
  }
}

/// Reads `permissions.policies` into one draft per mode.
List<PermissionPolicyDraft> permissionPolicyDrafts(Object? policies) {
  int? read(Object? policy, String key) {
    if (policy is! Map) return null;
    final v = policy[key];
    return v is num && v >= 1 && v <= 10 ? v.toInt() : null;
  }

  return [
    for (final d in permissionPolicyDefaults)
      PermissionPolicyDraft(
        d,
        askAt: d.askAt == null
            ? null
            : read(policies is Map ? policies[d.mode] : null, 'askAt'),
        cancelAt: read(policies is Map ? policies[d.mode] : null, 'cancelAt'),
      ),
  ];
}

/// The `permissions.policies` object of a plane settings patch, or null when
/// every mode keeps its built-in thresholds.
Map<String, Object?>? permissionPoliciesJson(
  List<PermissionPolicyDraft> drafts,
) {
  final out = <String, Object?>{
    for (final d in drafts)
      if (d.toJson() case final json?) d.defaults.mode: json,
  };
  return out.isEmpty ? null : out;
}

/// Editor for the plane-wide mode thresholds: per mode, the score asking
/// starts at and the score calls are cancelled at.
class PermissionPoliciesEditor extends StatelessWidget {
  const PermissionPoliciesEditor({
    super.key,
    required this.drafts,
    required this.onChanged,
  });

  final List<PermissionPolicyDraft> drafts;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          'Below "Ask from" a call runs; from it the user is asked; from '
          '"Cancel from" the call is cancelled without asking. Setting both '
          'to the same score never asks. A session that has read web content '
          'asks from 5 at the latest, in every mode but Full access.',
          style: TextStyle(color: tokens.textSecondary, fontSize: 12),
        ),
        const SizedBox(height: 8),
        for (final d in drafts)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: _PolicyRow(draft: d, onChanged: onChanged),
          ),
      ],
    );
  }
}

class _PolicyRow extends StatelessWidget {
  const _PolicyRow({required this.draft, required this.onChanged});

  final PermissionPolicyDraft draft;
  final VoidCallback onChanged;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final mode = draft.defaults.mode;
    final full = draft.defaults.askAt == null;
    return Wrap(
      spacing: 12,
      runSpacing: 8,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        SizedBox(
          width: 160,
          child: Text(
            draft.defaults.name,
            style: TextStyle(
              fontWeight: FontWeight.w500,
              color: full ? tokens.error : tokens.textPrimary,
            ),
          ),
        ),
        SizedBox(
          width: 152,
          child: full
              ? InputDecorator(
                  decoration: const InputDecoration(
                    labelText: 'Ask from',
                    border: OutlineInputBorder(),
                    isDense: true,
                    enabled: false,
                  ),
                  child: Text(
                    'Never',
                    style: TextStyle(color: tokens.textMuted),
                  ),
                )
              : _scoreDropdown(
                  key: Key('policy-ask-$mode'),
                  label: 'Ask from',
                  value: draft.askAt!,
                  defaultValue: draft.defaults.askAt,
                  error: draft.isValid ? null : 'Above cancel',
                  onChanged: (v) {
                    draft.askAt = v;
                    onChanged();
                  },
                ),
        ),
        SizedBox(
          width: 152,
          child: _scoreDropdown(
            key: Key('policy-cancel-$mode'),
            label: 'Cancel from',
            value: draft.cancelAt,
            defaultValue: draft.defaults.cancelAt,
            onChanged: (v) {
              draft.cancelAt = v;
              onChanged();
            },
          ),
        ),
        if (!full && draft.isValid && !draft.asks)
          Text(
            'Never asks',
            key: Key('policy-never-asks-$mode'),
            style: TextStyle(color: tokens.warning, fontSize: 12),
          ),
        if (draft.isModified)
          TextButton(
            key: Key('policy-reset-$mode'),
            onPressed: () {
              draft.reset();
              onChanged();
            },
            child: const Text('Reset to default'),
          ),
      ],
    );
  }

  Widget _scoreDropdown({
    required Key key,
    required String label,
    required int value,
    required int? defaultValue,
    required ValueChanged<int> onChanged,
    String? error,
  }) {
    return DropdownButtonFormField<int>(
      key: key,
      initialValue: value,
      isExpanded: true,
      decoration: InputDecoration(
        labelText: label,
        border: const OutlineInputBorder(),
        isDense: true,
        errorText: error,
      ),
      items: [
        for (var r = 1; r <= 10; r++)
          DropdownMenuItem(
            value: r,
            child: Text(
              '$r · ${permissionBandLabel(r)}'
              '${r == defaultValue ? ' (default)' : ''}',
              overflow: TextOverflow.ellipsis,
            ),
          ),
      ],
      onChanged: (v) {
        if (v != null) onChanged(v);
      },
    );
  }
}
