final defaultCatalogBase = Uri.parse('http://localhost:8080');

class CatalogException implements Exception {
  CatalogException({required this.statusCode, required this.message});

  final int statusCode;
  final String message;

  @override
  String toString() => 'CatalogException($statusCode): $message';
}

class ModelInfo {
  const ModelInfo({required this.id, required this.name});

  final String id;
  final String name;

  factory ModelInfo.fromJson(Map<String, dynamic> json) {
    return ModelInfo(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? json['id'] as String? ?? '',
    );
  }
}

class Provider {
  const Provider({
    required this.id,
    required this.name,
    required this.type,
    required this.baseUrl,
    required this.apiKey,
    required this.models,
    this.modelsUpdatedAt,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final String type;
  final String baseUrl;
  final String apiKey;
  final List<ModelInfo> models;
  final DateTime? modelsUpdatedAt;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory Provider.fromJson(Map<String, dynamic> json) {
    final modelsJson = json['models'];
    return Provider(
      id: json['id'] as String,
      name: json['name'] as String,
      type: json['type'] as String,
      baseUrl: json['baseUrl'] as String,
      apiKey: json['apiKey'] as String? ?? '',
      models: modelsJson is List
          ? modelsJson
                .cast<Map<String, dynamic>>()
                .map(ModelInfo.fromJson)
                .toList()
          : const [],
      modelsUpdatedAt: _parseDate(json['modelsUpdatedAt']),
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
}

class ThreadSummary {
  const ThreadSummary({
    required this.id,
    required this.title,
    required this.titleSource,
    this.agentId,
    this.currentModel,
    this.messageCount = 0,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String title;
  final String titleSource;
  final String? agentId;
  final String? currentModel;
  final int messageCount;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ThreadSummary.fromJson(Map<String, dynamic> json) {
    return ThreadSummary(
      id: json['id'] as String,
      title: json['title'] as String,
      titleSource: json['titleSource'] as String,
      agentId: json['agentId'] as String?,
      currentModel: json['currentModel'] as String?,
      messageCount: json['messageCount'] as int? ?? 0,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
}

class ThreadMessage {
  const ThreadMessage({
    required this.id,
    required this.role,
    required this.content,
    required this.position,
    required this.createdAt,
  });

  final String id;
  final String role;
  final String content;
  final int position;
  final DateTime createdAt;

  factory ThreadMessage.fromJson(Map<String, dynamic> json) {
    return ThreadMessage(
      id: json['id'] as String,
      role: json['role'] as String,
      content: json['content'] as String,
      position: json['position'] as int,
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

class ThreadDetail {
  const ThreadDetail({required this.thread, required this.messages});

  final ThreadSummary thread;
  final List<ThreadMessage> messages;

  String? get agentId => thread.agentId;

  factory ThreadDetail.fromJson(Map<String, dynamic> json) {
    return ThreadDetail(
      thread: ThreadSummary.fromJson(json),
      messages: (json['messages'] as List? ?? const [])
          .cast<Map<String, dynamic>>()
          .map(ThreadMessage.fromJson)
          .toList(),
    );
  }
}

class Agent {
  const Agent({
    required this.id,
    required this.name,
    this.description = '',
    required this.version,
    required this.providerId,
    required this.defaultModel,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final String description;
  final int version;
  final String providerId;
  final String defaultModel;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory Agent.fromJson(Map<String, dynamic> json) {
    return Agent(
      id: json['id'] as String,
      name: json['name'] as String,
      description: json['description'] as String? ?? '',
      version: json['version'] as int? ?? 0,
      providerId: json['providerId'] as String,
      defaultModel: json['defaultModel'] as String,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
}

DateTime? _parseDate(Object? value) {
  if (value is! String || value.isEmpty) {
    return null;
  }
  return DateTime.parse(value);
}
