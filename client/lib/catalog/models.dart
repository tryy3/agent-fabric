import 'dart:typed_data';

import '../acp/agent_connection.dart';

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

const _unset = Object();

class Project {
  const Project({
    required this.id,
    required this.name,
    this.description = '',
    this.settings = const {},
    this.remotes = const [],
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final String description;
  final Map<String, dynamic> settings;
  final List<dynamic> remotes;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory Project.fromJson(Map<String, dynamic> json) {
    final settings = json['settings'];
    final remotes = json['remotes'];
    return Project(
      id: json['id'] as String,
      name: json['name'] as String,
      description: json['description'] as String? ?? '',
      settings: _stringKeyMap(settings),
      remotes: remotes is List ? List<dynamic>.from(remotes) : const [],
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
}

class Resource {
  const Resource({
    required this.id,
    required this.name,
    required this.kind,
    this.spec = const {},
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final String kind;
  final Map<String, dynamic> spec;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory Resource.fromJson(Map<String, dynamic> json) {
    return Resource(
      id: json['id'] as String,
      name: json['name'] as String,
      kind: json['kind'] as String? ?? 'container',
      spec: _stringKeyMap(json['spec']),
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
    this.viewModeId,
    this.projectId = '',
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String title;
  final String titleSource;
  final String? agentId;
  final String? currentModel;
  final int messageCount;
  final String? viewModeId;
  final String projectId;
  final DateTime createdAt;
  final DateTime updatedAt;

  ThreadSummary copyWith({
    String? id,
    String? title,
    String? titleSource,
    String? agentId,
    String? currentModel,
    int? messageCount,
    DateTime? createdAt,
    DateTime? updatedAt,
    Object? viewModeId = _unset,
    String? projectId,
  }) {
    return ThreadSummary(
      id: id ?? this.id,
      title: title ?? this.title,
      titleSource: titleSource ?? this.titleSource,
      agentId: agentId ?? this.agentId,
      currentModel: currentModel ?? this.currentModel,
      messageCount: messageCount ?? this.messageCount,
      createdAt: createdAt ?? this.createdAt,
      updatedAt: updatedAt ?? this.updatedAt,
      viewModeId: identical(viewModeId, _unset)
          ? this.viewModeId
          : viewModeId as String?,
      projectId: projectId ?? this.projectId,
    );
  }

  factory ThreadSummary.fromJson(Map<String, dynamic> json) {
    return ThreadSummary(
      id: json['id'] as String,
      title: json['title'] as String,
      titleSource: json['titleSource'] as String,
      agentId: json['agentId'] as String?,
      currentModel: json['currentModel'] as String?,
      messageCount: json['messageCount'] as int? ?? 0,
      viewModeId: json['viewModeId'] as String?,
      projectId: json['projectId'] as String? ?? '',
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
    this.thought,
    this.model,
    this.providerName,
    this.stopReason,
    this.usage,
    this.toolCalls = const [],
    this.activities = const [],
  });

  final String id;
  final String role;
  final String content;
  final int position;
  final DateTime createdAt;
  final String? thought;
  final String? model;
  final String? providerName;
  final String? stopReason;
  final TurnUsage? usage;
  final List<ThreadToolCall> toolCalls;

  /// Ordered thought / tool_call activities from `parts` (event order).
  final List<ThreadActivity> activities;

  factory ThreadMessage.fromJson(Map<String, dynamic> json) {
    String? thought;
    TurnUsage? usage;
    final toolCalls = <ThreadToolCall>[];
    final activities = <ThreadActivity>[];
    final parts = json['parts'];
    if (parts is List) {
      for (final raw in parts) {
        if (raw is! Map) {
          continue;
        }
        final part = Map<String, dynamic>.from(raw);
        switch (part['type']) {
          case 'thought':
            final text = part['text'] as String? ?? '';
            if (text.isEmpty) {
              break;
            }
            thought = thought == null ? text : '$thought$text';
            activities.add(ThreadActivity.thought(text));
          case 'usage':
            usage = _usageFromPart(part);
          case 'tool_call':
            final tool = ThreadToolCall.fromJson(part);
            toolCalls.add(tool);
            activities.add(ThreadActivity.toolCall(tool));
        }
      }
    }
    return ThreadMessage(
      id: json['id'] as String,
      role: json['role'] as String,
      content: json['content'] as String,
      position: json['position'] as int,
      createdAt: DateTime.parse(json['createdAt'] as String),
      thought: thought,
      model: json['model'] as String?,
      providerName: json['providerName'] as String?,
      stopReason: json['stopReason'] as String?,
      usage: usage,
      toolCalls: toolCalls,
      activities: activities,
    );
  }
}

sealed class ThreadActivity {
  const ThreadActivity();
  const factory ThreadActivity.thought(String text) = ThreadThoughtActivity;
  const factory ThreadActivity.toolCall(ThreadToolCall toolCall) =
      ThreadToolCallActivity;
}

final class ThreadThoughtActivity extends ThreadActivity {
  const ThreadThoughtActivity(this.text);
  final String text;
}

final class ThreadToolCallActivity extends ThreadActivity {
  const ThreadToolCallActivity(this.toolCall);
  final ThreadToolCall toolCall;
}

class ThreadToolCall {
  const ThreadToolCall({
    required this.id,
    required this.title,
    this.status,
    this.input,
    this.output,
  });

  final String id;
  final String title;
  final String? status;
  final Object? input;
  final Object? output;

  factory ThreadToolCall.fromJson(Map<String, dynamic> json) {
    return ThreadToolCall(
      id: json['toolCallId'] as String? ?? '',
      title: json['title'] as String? ?? json['name'] as String? ?? 'Tool call',
      status: json['status'] as String?,
      input: json['input'],
      output: json['output'],
    );
  }
}

class ThreadDetail {
  const ThreadDetail({required this.thread, required this.messages});

  final ThreadSummary thread;
  final List<ThreadMessage> messages;

  String? get agentId => thread.agentId;

  factory ThreadDetail.fromJson(Map<String, dynamic> json) {
    final messages = (json['messages'] as List? ?? const [])
        .cast<Map<String, dynamic>>()
        .map(ThreadMessage.fromJson)
        .toList();
    return ThreadDetail(
      thread: ThreadSummary.fromJson({
        ...json,
        'messageCount': json['messageCount'] as int? ?? messages.length,
      }),
      messages: messages,
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
    this.providerName,
    required this.defaultModel,
    this.settings = const {},
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final String description;
  final int version;
  final String? providerId;
  final String? providerName;
  final String? defaultModel;
  final Map<String, dynamic> settings;
  final DateTime createdAt;
  final DateTime updatedAt;

  bool get isComplete =>
      providerId != null &&
      providerId!.isNotEmpty &&
      defaultModel != null &&
      defaultModel!.isNotEmpty;

  factory Agent.fromJson(Map<String, dynamic> json) {
    final settings = json['settings'];
    return Agent(
      id: json['id'] as String,
      name: json['name'] as String,
      description: json['description'] as String? ?? '',
      version: json['version'] as int? ?? 0,
      providerId: json['providerId'] as String?,
      providerName: json['providerName'] as String?,
      defaultModel: json['defaultModel'] as String?,
      settings: settings is Map<String, dynamic> ? settings : const {},
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
}

class FsEntry {
  const FsEntry({
    required this.name,
    required this.isDir,
    this.size = 0,
    this.modTime,
  });

  final String name;
  final bool isDir;
  final int size;
  final DateTime? modTime;

  factory FsEntry.fromJson(Map<String, dynamic> json) {
    return FsEntry(
      name: json['name'] as String? ?? '',
      isDir: json['isDir'] as bool? ?? false,
      size: json['size'] as int? ?? 0,
      modTime: _parseDate(json['modTime']),
    );
  }
}

class FsListing {
  const FsListing({required this.path, required this.entries});

  final String path;
  final List<FsEntry> entries;

  factory FsListing.fromJson(Map<String, dynamic> json) {
    final entries = json['entries'];
    return FsListing(
      path: json['path'] as String? ?? '/',
      entries: entries is List
          ? [
              for (final e in entries)
                if (e is Map<String, dynamic>) FsEntry.fromJson(e),
            ]
          : const [],
    );
  }
}

class GitCommit {
  const GitCommit({
    required this.sha,
    required this.message,
    this.committedAt,
    this.checkpointId,
    this.label,
  });

  final String sha;
  final String message;
  final DateTime? committedAt;
  final String? checkpointId;
  final String? label;

  factory GitCommit.fromJson(Map<String, dynamic> json) {
    return GitCommit(
      sha: json['sha'] as String? ?? '',
      message: json['message'] as String? ?? '',
      committedAt: _parseDate(json['committedAt']),
      checkpointId: json['checkpointId'] as String?,
      label: json['label'] as String?,
    );
  }
}

class Checkpoint {
  const Checkpoint({
    required this.id,
    required this.projectId,
    required this.sha,
    required this.label,
    this.threadId,
    this.messageId,
    required this.createdAt,
  });

  final String id;
  final String projectId;
  final String sha;
  final String label;
  final String? threadId;
  final String? messageId;
  final DateTime createdAt;

  factory Checkpoint.fromJson(Map<String, dynamic> json) {
    return Checkpoint(
      id: json['id'] as String? ?? '',
      projectId: json['projectId'] as String? ?? '',
      sha: json['sha'] as String? ?? '',
      label: json['label'] as String? ?? '',
      threadId: json['threadId'] as String?,
      messageId: json['messageId'] as String?,
      createdAt:
          _parseDate(json['createdAt']) ??
          DateTime.fromMillisecondsSinceEpoch(0),
    );
  }
}

class DiffResult {
  const DiffResult({required this.from, required this.to, required this.diff});

  final String from;
  final String to;
  final String diff;

  factory DiffResult.fromJson(Map<String, dynamic> json) {
    return DiffResult(
      from: json['from'] as String? ?? '',
      to: json['to'] as String? ?? '',
      diff: json['diff'] as String? ?? '',
    );
  }
}

class ExportMethod {
  const ExportMethod({
    required this.id,
    required this.label,
    this.enabled = false,
    this.reason,
  });

  final String id;
  final String label;
  final bool enabled;
  final String? reason;

  static const defaults = [
    ExportMethod(id: 'download', label: 'Download zip', enabled: true),
    ExportMethod(
      id: 'github',
      label: 'GitHub',
      enabled: false,
      reason: 'coming soon',
    ),
  ];

  factory ExportMethod.fromJson(Map<String, dynamic> json) {
    return ExportMethod(
      id: json['id'] as String? ?? '',
      label: json['label'] as String? ?? json['id'] as String? ?? '',
      enabled: json['enabled'] as bool? ?? false,
      reason: json['reason'] as String?,
    );
  }
}

class ExportArchive {
  const ExportArchive({
    required this.filename,
    required this.bytes,
    this.mediaType = 'application/zip',
  });

  final String filename;
  final Uint8List bytes;
  final String mediaType;
}

class PlaneSettings {
  const PlaneSettings({this.sandbox = const {}, this.environment = const {}});

  final Map<String, dynamic> sandbox;
  final Map<String, dynamic> environment;

  factory PlaneSettings.fromJson(Map<String, dynamic> json) {
    return PlaneSettings(
      sandbox: _stringKeyMap(json['sandbox']),
      environment: _stringKeyMap(json['environment']),
    );
  }
}

Map<String, dynamic> _stringKeyMap(Object? value) {
  if (value is Map<String, dynamic>) {
    return Map<String, dynamic>.from(value);
  }
  if (value is Map) {
    return Map<String, dynamic>.from(value);
  }
  return const {};
}

DateTime? _parseDate(Object? value) {
  if (value is! String || value.isEmpty) {
    return null;
  }
  return DateTime.parse(value);
}

TurnUsage _usageFromPart(Map<String, dynamic> part) {
  return TurnUsage(
    promptTokens: _asInt(part['promptTokens']),
    completionTokens: _asInt(part['completionTokens']),
    totalTokens: _asInt(part['totalTokens']),
    ttftMs: _asInt(part['ttftMs']),
    elapsedMs: _asInt(part['elapsedMs']),
    promptMs: _asDouble(part['promptMs']),
    predictedMs: _asDouble(part['predictedMs']),
    promptPerSecond: _asDouble(part['promptPerSecond']),
    predictedPerSecond: _asDouble(part['predictedPerSecond']),
    deltas: _asInt(part['deltas']),
    stopReason: part['stopReason'] as String?,
    extras: {
      for (final entry in part.entries)
        if (!kTurnUsageKnownKeys.contains(entry.key))
          entry.key: entry.value as Object?,
    },
  );
}

int? _asInt(Object? value) {
  if (value is num) {
    return value.toInt();
  }
  return null;
}

double? _asDouble(Object? value) {
  if (value is num) {
    return value.toDouble();
  }
  return null;
}
