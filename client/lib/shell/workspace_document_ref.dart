import '../workspace/open_with.dart';

/// Durable reference to an open document view for one project.
class WorkspaceDocumentRef {
  const WorkspaceDocumentRef({
    required this.path,
    required this.appId,
    this.viewMode = EditorViewMode.code,
    this.focused = false,
  });

  final String path;
  final WorkspaceAppId appId;
  final EditorViewMode viewMode;
  final bool focused;

  Map<String, dynamic> toJson() => {
    'path': path,
    'appId': appId.name,
    if (viewMode != EditorViewMode.code) 'viewMode': viewMode.name,
    if (focused) 'focused': true,
  };

  factory WorkspaceDocumentRef.fromJson(Map<String, dynamic> json) {
    final path = json['path'];
    final appRaw = json['appId'];
    if (path is! String || path.isEmpty || appRaw is! String) {
      throw const FormatException('invalid WorkspaceDocumentRef');
    }
    final app = WorkspaceAppId.values.asNameMap()[appRaw];
    if (app == null) {
      throw FormatException('unknown appId: $appRaw');
    }
    final modeRaw = json['viewMode'];
    final mode = modeRaw is String
        ? (EditorViewMode.values.asNameMap()[modeRaw] ?? EditorViewMode.code)
        : EditorViewMode.code;
    return WorkspaceDocumentRef(
      path: path,
      appId: app,
      viewMode: mode,
      focused: json['focused'] == true,
    );
  }
}
