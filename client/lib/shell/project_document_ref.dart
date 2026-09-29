import '../workspace/open_with.dart';

/// Durable reference to an open document view for one project.
class ProjectDocumentRef {
  const ProjectDocumentRef({
    required this.path,
    required this.appId,
    this.viewMode = EditorViewMode.code,
    this.focused = false,
  });

  final String path;
  final ProjectFileAppId appId;
  final EditorViewMode viewMode;
  final bool focused;

  Map<String, dynamic> toJson() => {
    'path': path,
    'appId': appId.name,
    if (viewMode != EditorViewMode.code) 'viewMode': viewMode.name,
    if (focused) 'focused': true,
  };

  factory ProjectDocumentRef.fromJson(Map<String, dynamic> json) {
    final path = json['path'];
    final appRaw = json['appId'];
    if (path is! String || path.isEmpty || appRaw is! String) {
      throw const FormatException('invalid ProjectDocumentRef');
    }
    final app = ProjectFileAppId.values.asNameMap()[appRaw];
    if (app == null) {
      throw FormatException('unknown appId: $appRaw');
    }
    final modeRaw = json['viewMode'];
    final mode = modeRaw is String
        ? (EditorViewMode.values.asNameMap()[modeRaw] ?? EditorViewMode.code)
        : EditorViewMode.code;
    return ProjectDocumentRef(
      path: path,
      appId: app,
      viewMode: mode,
      focused: json['focused'] == true,
    );
  }
}
