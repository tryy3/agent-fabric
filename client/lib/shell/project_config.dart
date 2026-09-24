/// Reads project settings the context bar can show without new APIs.
class ProjectConfigSummary {
  const ProjectConfigSummary({
    this.tools = const [],
    this.mcpServers = const [],
    this.resourceId,
  });

  final List<String> tools;
  final List<String> mcpServers;
  final String? resourceId;

  factory ProjectConfigSummary.fromSettings(Map<String, dynamic> settings) {
    return ProjectConfigSummary(
      tools: _stringList(settings['tools'], 'allow'),
      mcpServers: _mcpNames(settings['mcp']),
      resourceId: _resourceId(settings['environment']),
    );
  }

  static List<String> _stringList(Object? bucket, String key) {
    if (bucket is! Map) {
      return const [];
    }
    final raw = bucket[key];
    if (raw is! List) {
      return const [];
    }
    return [
      for (final item in raw)
        if (item is String && item.trim().isNotEmpty) item.trim(),
    ];
  }

  static List<String> _mcpNames(Object? bucket) {
    if (bucket is! Map) {
      return const [];
    }
    final raw = bucket['servers'];
    if (raw is! List) {
      return const [];
    }
    final names = <String>[];
    for (final item in raw) {
      if (item is String && item.trim().isNotEmpty) {
        names.add(item.trim());
      } else if (item is Map) {
        final name = item['name'];
        if (name is String && name.trim().isNotEmpty) {
          names.add(name.trim());
        }
      }
    }
    return names;
  }

  static String? _resourceId(Object? environment) {
    if (environment is! Map) {
      return null;
    }
    final id = environment['resourceId'];
    if (id is String && id.isNotEmpty) {
      return id;
    }
    return null;
  }
}

/// Label for the environment chip. Falls back to "Environment".
String environmentLabel({
  Map<String, dynamic>? resolved,
  String? resourceName,
  String? resourceKind,
}) {
  final resource = resolved?['resource'];
  if (resource is Map) {
    final name = resource['name'];
    final kind = resource['kind'];
    final resolvedName = name is String && name.isNotEmpty ? name : null;
    final resolvedKind = kind is String && kind.isNotEmpty ? kind : null;
    if (resolvedKind != null && resolvedName != null) {
      return '$resolvedKind · $resolvedName';
    }
    if (resolvedName != null) {
      return resolvedName;
    }
  }
  if (resourceKind != null &&
      resourceKind.isNotEmpty &&
      resourceName != null &&
      resourceName.isNotEmpty) {
    return '$resourceKind · $resourceName';
  }
  if (resourceName != null && resourceName.isNotEmpty) {
    return resourceName;
  }
  return 'Environment';
}

const projectMarkerColors = <int>[
  0xFF22D3D1,
  0xFFF59E42,
  0xFF35C97A,
  0xFF38A5FF,
  0xFFF06464,
];

int projectMarkerColor(String id) {
  if (id.isEmpty) {
    return projectMarkerColors.first;
  }
  return projectMarkerColors[id.hashCode.abs() % projectMarkerColors.length];
}
