/// Pure helpers for project-relative paths (no leading slash, `.` is the root).
///
/// Shared by the file tree, controller remapping and any future command
/// palette so path rules live in one place.
library;

const String projectRootPath = '.';

bool isProjectRoot(String path) =>
    path.isEmpty || path == '/' || path == projectRootPath;

String joinProjectPath(String dir, String name) =>
    isProjectRoot(dir) ? name : '$dir/$name';

String parentOfPath(String path) {
  final i = path.lastIndexOf('/');
  return i <= 0 ? projectRootPath : path.substring(0, i);
}

String baseNameOfPath(String path) {
  final i = path.lastIndexOf('/');
  return i < 0 ? path : path.substring(i + 1);
}

/// True when [path] is [ancestor] itself or lives beneath it.
bool isSameOrDescendant(String path, String ancestor) =>
    path == ancestor || path.startsWith('$ancestor/');

/// Rewrites [path] after [from] was moved to [to]. Paths outside the moved
/// subtree are returned unchanged.
String remapPath(String path, String from, String to) {
  if (path == from) {
    return to;
  }
  if (path.startsWith('$from/')) {
    return '$to${path.substring(from.length)}';
  }
  return path;
}

/// Length of the part of [name] a rename should pre-select: everything before
/// the last dot, keeping dotfiles (`.env`) whole.
int renameSelectionLength(String name) {
  final dot = name.lastIndexOf('.');
  return dot <= 0 ? name.length : dot;
}

/// Returns an operator-facing message when [name] is not a valid single path
/// segment, or null when it is fine.
String? validateEntryName(String name) {
  final trimmed = name.trim();
  if (trimmed.isEmpty) {
    return 'Enter a name.';
  }
  if (trimmed == '.' || trimmed == '..') {
    return 'That name is not allowed.';
  }
  if (trimmed.contains('/') || trimmed.contains(r'\')) {
    return 'Names cannot contain slashes.';
  }
  return null;
}
