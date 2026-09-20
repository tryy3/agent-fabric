import 'package:flutter/foundation.dart';

/// Widget ↔ save/read/dirty/language. No re_editor types.
abstract class TextEditorSession implements Listenable {
  String get path;
  String get languageId;
  bool get isDirty;
  String get text;

  /// Called by the editor view on user edits. Marks the FileDocument dirty.
  void handleTextChanged(String text);

  /// PUT FileDocument.bytes. Workspace-level ⌘/Ctrl+S calls this.
  Future<void> save();

  /// GET and replace bytes. No-ops (keeps buffer, sets diskChanged) if dirty
  /// unless [force] is true.
  Future<void> reload({bool force = false});
}
