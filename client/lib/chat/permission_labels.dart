import 'package:acpd/acpd.dart';

/// User-visible label for a permission option.
///
/// Session-scoped grants are labeled **Allow for this session** even when the
/// ACP kind remains [PermissionOptionKind.allowAlways] or the wire id is still
/// the legacy `allow_always`.
String permissionOptionLabel(PermissionOption option) {
  if (option.kind == PermissionOptionKind.allowAlways ||
      option.optionId == 'allow_session' ||
      option.optionId == 'allow_always') {
    return 'Allow for this session';
  }
  return option.name;
}
