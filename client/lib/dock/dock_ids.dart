import '../workspace/open_with.dart';

abstract final class DockIds {
  static const threads = 'threads';
  static const files = 'files';
  static const chat = 'chat';

  static String doc(String path, WorkspaceAppId app) => 'doc:$path:${app.name}';

  static bool isDoc(dynamic id) => id is String && id.startsWith('doc:');
}
