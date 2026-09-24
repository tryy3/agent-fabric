import '../workspace/open_with.dart';

/// Path and app decoded from a [DockIds.doc] id.
class DockDocId {
  const DockDocId({required this.path, required this.appId});

  final String path;
  final WorkspaceAppId appId;
}

abstract final class DockIds {
  static const threads = 'threads';
  static const files = 'files';
  static const chat = 'chat';

  static String doc(String path, WorkspaceAppId app) => 'doc:$path:${app.name}';

  static bool isDoc(dynamic id) => id is String && id.startsWith('doc:');

  /// Parses `doc:<path>:<appName>`. Returns null when the id is not a doc id.
  static DockDocId? parseDoc(dynamic id) {
    if (id is! String || !id.startsWith('doc:')) {
      return null;
    }
    final body = id.substring(4);
    final sep = body.lastIndexOf(':');
    if (sep <= 0 || sep == body.length - 1) {
      return null;
    }
    final path = body.substring(0, sep);
    final app = WorkspaceAppId.values.asNameMap()[body.substring(sep + 1)];
    if (path.isEmpty || app == null) {
      return null;
    }
    return DockDocId(path: path, appId: app);
  }

  /// Title-case label for a core dock tab. Document tabs keep [OpenView.tabLabel].
  static String coreTitle(dynamic id) {
    if (id == threads) return 'Threads';
    if (id == files) return 'Files';
    if (id == chat) return 'Chat';
    return id?.toString() ?? '';
  }
}
