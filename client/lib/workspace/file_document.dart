import 'dart:convert';

import 'package:flutter/foundation.dart';

String languageIdForPath(String path) {
  final name = path.toLowerCase();
  if (name.endsWith('.html') || name.endsWith('.htm')) {
    return 'html';
  }
  if (name.endsWith('.js') || name.endsWith('.mjs')) {
    return 'javascript';
  }
  if (name.endsWith('.css')) {
    return 'css';
  }
  if (name.endsWith('.json')) {
    return 'json';
  }
  return 'plaintext';
}

/// Canonical catalog bytes for one path. Owned by WorkspaceController.
class FileDocument extends ChangeNotifier {
  FileDocument({
    required this.projectId,
    required this.path,
    required Uint8List bytes,
  }) : _bytes = bytes,
       _savedBytes = Uint8List.fromList(bytes);

  final String projectId;
  final String path;

  Uint8List _bytes;
  Uint8List _savedBytes;
  bool _dirty = false;
  bool _diskChanged = false;
  int _revision = 0;

  Uint8List get bytes => _bytes;
  bool get isDirty => _dirty;
  bool get diskChanged => _diskChanged;
  String get languageId => languageIdForPath(path);

  /// Bumps whenever the catalog's copy of this path is known to have changed
  /// (save, clean reload, or a dirty buffer left behind by a disk change).
  /// Previews key on it to reload the served bytes.
  int get revision => _revision;

  bool get isUtf8 {
    try {
      utf8.decode(_bytes);
      return true;
    } on FormatException {
      return false;
    }
  }

  /// Throws [FormatException] if not UTF-8.
  String get text => utf8.decode(_bytes);

  void replaceBytes(Uint8List next, {required bool markDirty}) {
    _bytes = next;
    if (markDirty) {
      _dirty = !listEquals(_bytes, _savedBytes);
    } else {
      _savedBytes = Uint8List.fromList(next);
      _dirty = false;
      _diskChanged = false;
      _revision++;
    }
    notifyListeners();
  }

  void replaceText(String next) =>
      replaceBytes(Uint8List.fromList(utf8.encode(next)), markDirty: true);

  void markClean() {
    if (_dirty || _diskChanged) {
      _revision++;
    }
    _savedBytes = Uint8List.fromList(_bytes);
    _dirty = false;
    _diskChanged = false;
    notifyListeners();
  }

  void markDiskChanged() {
    if (!_diskChanged) {
      _revision++;
    }
    _diskChanged = true;
    notifyListeners();
  }
}
