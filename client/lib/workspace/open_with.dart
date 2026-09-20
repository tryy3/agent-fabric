enum WorkspaceAppId {
  textEditor,
  webPreview,
  imagePreview,
  audioPreview,
  download,
}

class OpenView {
  const OpenView({
    required this.viewId,
    required this.path,
    required this.appId,
  });

  final String viewId;
  final String path;
  final WorkspaceAppId appId;

  String get tabLabel {
    final name = path.split('/').where((p) => p.isNotEmpty).last;
    return '$name · ${appLabel(appId)}';
  }
}

class EditorGroup {
  EditorGroup({required this.groupId, List<OpenView>? tabs, this.activeViewId})
    : tabs = tabs ?? [];

  final String groupId;
  final List<OpenView> tabs;
  String? activeViewId;

  OpenView? get active {
    final id = activeViewId;
    if (id == null) {
      return null;
    }
    for (final tab in tabs) {
      if (tab.viewId == id) {
        return tab;
      }
    }
    return tabs.isEmpty ? null : tabs.last;
  }
}

class FileAssociation {
  const FileAssociation({
    required this.extension,
    required this.apps,
    this.userDefault,
  });

  final String extension;
  final List<WorkspaceAppId> apps;
  final WorkspaceAppId? userDefault;

  WorkspaceAppId? get defaultApp =>
      userDefault ?? (apps.isEmpty ? null : apps.first);
}

String appLabel(WorkspaceAppId id) {
  switch (id) {
    case WorkspaceAppId.textEditor:
      return 'Editor';
    case WorkspaceAppId.webPreview:
      return 'Web preview';
    case WorkspaceAppId.imagePreview:
      return 'Image';
    case WorkspaceAppId.audioPreview:
      return 'Audio';
    case WorkspaceAppId.download:
      return 'Download';
  }
}

String extensionOf(String path) {
  final name = path.split('/').where((p) => p.isNotEmpty).last;
  final dot = name.lastIndexOf('.');
  if (dot <= 0 || dot == name.length - 1) {
    return '';
  }
  return name.substring(dot + 1).toLowerCase();
}

const defaultAssociations = <FileAssociation>[
  FileAssociation(
    extension: 'html',
    apps: [WorkspaceAppId.textEditor, WorkspaceAppId.webPreview],
  ),
  FileAssociation(
    extension: 'htm',
    apps: [WorkspaceAppId.textEditor, WorkspaceAppId.webPreview],
  ),
  FileAssociation(extension: 'css', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'js', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'mjs', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'json', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'md', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'txt', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'svg', apps: [WorkspaceAppId.textEditor]),
  FileAssociation(extension: 'png', apps: [WorkspaceAppId.imagePreview]),
  FileAssociation(extension: 'jpg', apps: [WorkspaceAppId.imagePreview]),
  FileAssociation(extension: 'jpeg', apps: [WorkspaceAppId.imagePreview]),
  FileAssociation(extension: 'gif', apps: [WorkspaceAppId.imagePreview]),
  FileAssociation(extension: 'webp', apps: [WorkspaceAppId.imagePreview]),
  FileAssociation(
    extension: 'mp3',
    apps: [WorkspaceAppId.audioPreview, WorkspaceAppId.download],
  ),
  FileAssociation(
    extension: 'wav',
    apps: [WorkspaceAppId.audioPreview, WorkspaceAppId.download],
  ),
  FileAssociation(
    extension: 'ogg',
    apps: [WorkspaceAppId.audioPreview, WorkspaceAppId.download],
  ),
];

FileAssociation associationFor(String path) {
  final ext = extensionOf(path);
  for (final assoc in defaultAssociations) {
    if (assoc.extension == ext) {
      return assoc;
    }
  }
  return const FileAssociation(extension: '', apps: [WorkspaceAppId.download]);
}

bool appCanOpen(WorkspaceAppId app, String path) {
  return associationFor(path).apps.contains(app);
}
