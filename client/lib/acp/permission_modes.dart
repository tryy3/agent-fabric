import 'package:acpd/acpd.dart' show SessionMode, SessionModeState;

/// The permission mode id of Full access: never asks, chosen per session only
/// and confirmed by the user each time (decision 24).
const kPermissionModeFull = 'full';

/// The permission mode an assistant uses when it stores none.
const kPermissionModeDefault = 'ask';

/// A permission mode the plane advertises for a session (an ACP session mode).
class PermissionModeOption {
  const PermissionModeOption({
    required this.id,
    required this.name,
    this.description,
  });

  final String id;
  final String name;

  /// The thresholds in words, as the plane pinned them for this session.
  final String? description;

  /// Whether this is Full access.
  bool get isFull => id == kPermissionModeFull;
}

/// Reads the permission modes of an ACP session mode state.
List<PermissionModeOption> permissionModesFrom(SessionModeState? state) => [
  for (final m in state?.availableModes ?? const <SessionMode>[])
    PermissionModeOption(id: m.id, name: m.name, description: m.description),
];

/// A session that can switch its permission mode. Kept apart from
/// AgentSessionApi so fakes that do not need modes stay small.
abstract interface class PermissionModeApi {
  /// The modes the plane offers for the current session, least to most
  /// permissive; empty before a session starts.
  List<PermissionModeOption> get permissionModes;

  /// The current session's mode id, or null without a session.
  String? get currentPermissionMode;

  /// Switches the current session to [modeId]. Full access needs
  /// [confirmFullAccess], set only after the user confirmed it.
  Future<void> setPermissionMode(
    String modeId, {
    bool confirmFullAccess = false,
  });
}
