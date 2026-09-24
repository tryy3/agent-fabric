import '../workspace/open_with.dart';

abstract final class DockIds {
  static const threads = 'threads';
  static const files = 'files';
  static const chat = 'chat';

  static String doc(String path, WorkspaceAppId app) => 'doc:$path:${app.name}';

  static bool isDoc(dynamic id) => id is String && id.startsWith('doc:');

  /// Title-case label for a core dock tab. Document tabs keep [OpenView.tabLabel].
  static String coreTitle(dynamic id) {
    if (id == threads) return 'Threads';
    if (id == files) return 'Files';
    if (id == chat) return 'Chat';
    return id?.toString() ?? '';
  }
}
