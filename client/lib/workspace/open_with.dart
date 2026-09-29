enum ProjectFileAppId {
  textEditor,
  webPreview,
  imagePreview,
  audioPreview,
  download,
}

/// How a text-editor document view combines source and rendered preview.
enum EditorViewMode { code, preview, split }

class OpenView {
  const OpenView({
    required this.viewId,
    required this.path,
    required this.appId,
  });

  final String viewId;
  final String path;
  final ProjectFileAppId appId;

  String get tabLabel {
    final name = path.split('/').where((p) => p.isNotEmpty).last;
    return '$name - ${appLabel(appId)}';
  }
}

class FileAssociation {
  const FileAssociation({
    required this.extension,
    required this.apps,
    this.userDefault,
  });

  final String extension;
  final List<ProjectFileAppId> apps;
  final ProjectFileAppId? userDefault;

  ProjectFileAppId? get defaultApp =>
      userDefault ?? (apps.isEmpty ? null : apps.first);
}

String appLabel(ProjectFileAppId id) {
  switch (id) {
    case ProjectFileAppId.textEditor:
      return 'Editor';
    case ProjectFileAppId.webPreview:
      return 'Web preview';
    case ProjectFileAppId.imagePreview:
      return 'Image';
    case ProjectFileAppId.audioPreview:
      return 'Audio';
    case ProjectFileAppId.download:
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
    apps: [ProjectFileAppId.textEditor, ProjectFileAppId.webPreview],
  ),
  FileAssociation(
    extension: 'htm',
    apps: [ProjectFileAppId.textEditor, ProjectFileAppId.webPreview],
  ),
  FileAssociation(extension: 'css', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'js', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'mjs', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'json', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'md', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'txt', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'svg', apps: [ProjectFileAppId.textEditor]),
  FileAssociation(extension: 'png', apps: [ProjectFileAppId.imagePreview]),
  FileAssociation(extension: 'jpg', apps: [ProjectFileAppId.imagePreview]),
  FileAssociation(extension: 'jpeg', apps: [ProjectFileAppId.imagePreview]),
  FileAssociation(extension: 'gif', apps: [ProjectFileAppId.imagePreview]),
  FileAssociation(extension: 'webp', apps: [ProjectFileAppId.imagePreview]),
  FileAssociation(
    extension: 'mp3',
    apps: [ProjectFileAppId.audioPreview, ProjectFileAppId.download],
  ),
  FileAssociation(
    extension: 'wav',
    apps: [ProjectFileAppId.audioPreview, ProjectFileAppId.download],
  ),
  FileAssociation(
    extension: 'ogg',
    apps: [ProjectFileAppId.audioPreview, ProjectFileAppId.download],
  ),
];

FileAssociation associationFor(String path) {
  final ext = extensionOf(path);
  for (final assoc in defaultAssociations) {
    if (assoc.extension == ext) {
      return assoc;
    }
  }
  return const FileAssociation(
    extension: '',
    apps: [ProjectFileAppId.download],
  );
}

bool appCanOpen(ProjectFileAppId app, String path) {
  return associationFor(path).apps.contains(app);
}
