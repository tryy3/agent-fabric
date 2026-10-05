import 'dart:typed_data';

import '../acp/agent_connection.dart';
import '../acp/gate_info.dart';

/// Local-dev catalog origin when `/config.json` is absent.
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

/// Inference connection type for OpenAI-compatible custom backends.
const providerTypeOpenAICompatible = 'openai_compatible';

/// Inference connection type for OpenCode Zen (pay-as-you-go).
const providerTypeOpenCodeZen = 'opencode_zen';

/// Inference connection type for OpenCode Go (subscription).
const providerTypeOpenCodeGo = 'opencode_go';

/// Inference connection type for Unsloth Studio (local, advanced sampling).
const providerTypeUnslothStudio = 'unsloth_studio';

/// Inference connection type for Berget AI (fixed EU Chat Completions endpoint).
const providerTypeBergetAI = 'berget_ai';

/// Whether [type] is an OpenCode Zen/Go family inference connection.
bool isOpenCodeProviderType(String type) =>
    type == providerTypeOpenCodeZen || type == providerTypeOpenCodeGo;

/// Whether [type] uses a plane-fixed base URL (no user-supplied endpoint).
bool hasFixedBaseUrl(String type) =>
    isOpenCodeProviderType(type) || type == providerTypeBergetAI;

/// Whether [type] exposes extended Chat Completions sampler knobs in Assistants.
bool supportsSamplerExtras(String type) =>
    type == providerTypeUnslothStudio || type == providerTypeBergetAI;

/// Human-readable label for an inference connection [type] string.
String providerTypeLabel(String type) {
  switch (type) {
    case providerTypeOpenCodeZen:
      return 'OpenCode Zen';
    case providerTypeOpenCodeGo:
      return 'OpenCode Go';
    case providerTypeUnslothStudio:
      return 'Unsloth Studio';
    case providerTypeBergetAI:
      return 'Berget AI';
    case providerTypeOpenAICompatible:
      return 'Custom';
    default:
      return type;
  }
}

/// Plane tool definition from `GET /v1/tools` (metadata only).
class ToolDefinition {
  const ToolDefinition({
    required this.name,
    required this.description,
    required this.parameters,
    required this.requires,
    required this.origin,
  });

  final String name;
  final String description;
  final Map<String, dynamic> parameters;
  final Map<String, dynamic> requires;
  final String origin;

  /// Raw catalog payload shown in the Tools status expand panel.
  Map<String, dynamic> get definitionJson => {
    'name': name,
    'description': description,
    'parameters': parameters,
    'requires': requires,
  };

  factory ToolDefinition.fromJson(Map<String, dynamic> json) {
    final rawParams = json['parameters'];
    final rawRequires = json['requires'];
    return ToolDefinition(
      name: json['name'] as String? ?? '',
      description: json['description'] as String? ?? '',
      parameters: rawParams is Map
          ? Map<String, dynamic>.from(rawParams)
          : const {},
      requires: rawRequires is Map
          ? Map<String, dynamic>.from(rawRequires)
          : const {},
      origin: json['origin'] as String? ?? '',
    );
  }
}

class InferenceConnection {
  const InferenceConnection({
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

  /// Whether this connection is OpenCode Zen or Go.
  bool get isOpenCode => isOpenCodeProviderType(type);

  /// Display label for [type].
  String get typeLabel => providerTypeLabel(type);

  factory InferenceConnection.fromJson(Map<String, dynamic> json) {
    final modelsJson = json['models'];
    return InferenceConnection(
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
    this.assistantId,
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
  final String? assistantId;
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
    String? assistantId,
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
      assistantId: assistantId ?? this.assistantId,
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
      assistantId: json['assistantId'] as String?,
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
    this.active = true,
    this.promptMessageId,
    this.status = 'completed',
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
  final List<TurnActivity> activities;

  /// Whether this message is on the active conversation path (inactive = superseded attempt).
  final bool active;

  /// For assistant attempts, the user message that started the turn.
  final String? promptMessageId;

  /// Attempt lifecycle: running, completed, failed, or cancelled.
  final String status;

  factory ThreadMessage.fromJson(Map<String, dynamic> json) {
    String? thought;
    TurnUsage? usage;
    final toolCalls = <ThreadToolCall>[];
    final activities = <TurnActivity>[];
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
            activities.add(TurnActivity.thought(text));
          case 'sent':
            final text = part['text'] as String? ?? '';
            if (text.isNotEmpty) {
              activities.add(TurnActivity.sent(text));
            }
          case 'usage':
            usage = _usageFromPart(part);
          case 'tool_call':
            final tool = ThreadToolCall.fromJson(part);
            toolCalls.add(tool);
            activities.add(TurnActivity.toolCall(tool));
          case 'error':
            final text = part['text'] as String? ?? '';
            if (text.isNotEmpty) {
              activities.add(TurnActivity.error(text));
            }
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
      active: json['active'] as bool? ?? true,
      promptMessageId: json['promptMessageId'] as String?,
      status: json['status'] as String? ?? 'completed',
    );
  }
}

sealed class TurnActivity {
  const TurnActivity();
  const factory TurnActivity.thought(String text) = TurnThoughtActivity;
  const factory TurnActivity.sent(String text) = TurnSentActivity;
  const factory TurnActivity.toolCall(ThreadToolCall toolCall) =
      TurnToolCallActivity;
  const factory TurnActivity.error(String text) = TurnErrorActivity;
}

final class TurnThoughtActivity extends TurnActivity {
  const TurnThoughtActivity(this.text);
  final String text;
}

final class TurnSentActivity extends TurnActivity {
  const TurnSentActivity(this.text);
  final String text;
}

final class TurnToolCallActivity extends TurnActivity {
  const TurnToolCallActivity(this.toolCall);
  final ThreadToolCall toolCall;
}

final class TurnErrorActivity extends TurnActivity {
  const TurnErrorActivity(this.text);
  final String text;
}

class ThreadToolCall {
  const ThreadToolCall({
    required this.id,
    required this.title,
    this.status,
    this.input,
    this.output,
    this.gate,
  });

  final String id;
  final String title;
  final String? status;
  final Object? input;
  final Object? output;
  final GateInfo? gate;

  factory ThreadToolCall.fromJson(Map<String, dynamic> json) {
    return ThreadToolCall(
      id: json['toolCallId'] as String? ?? '',
      title: json['title'] as String? ?? json['name'] as String? ?? 'Tool call',
      status: json['status'] as String?,
      input: json['input'],
      output: json['output'],
      gate: GateInfo.tryParse(json['gate']),
    );
  }
}

/// Scrubbed inter-service hop capture for the chat Inspector.
class HopCapture {
  const HopCapture({
    required this.id,
    required this.threadId,
    this.messageId,
    this.sessionId,
    required this.roundIndex,
    required this.hopKind,
    required this.direction,
    this.method,
    this.url,
    this.statusCode,
    required this.headers,
    required this.bodyText,
    required this.meta,
    required this.createdAt,
  });

  final String id;
  final String threadId;
  final String? messageId;
  final String? sessionId;
  final int roundIndex;
  final String hopKind;
  final String direction;
  final String? method;
  final String? url;
  final int? statusCode;
  final Map<String, dynamic> headers;
  final String bodyText;
  final Map<String, dynamic> meta;
  final DateTime createdAt;

  factory HopCapture.fromJson(Map<String, dynamic> json) {
    return HopCapture(
      id: json['id'] as String,
      threadId: json['threadId'] as String,
      messageId: json['messageId'] as String?,
      sessionId: json['sessionId'] as String?,
      roundIndex: json['roundIndex'] as int? ?? 0,
      hopKind: json['hopKind'] as String? ?? '',
      direction: json['direction'] as String? ?? '',
      method: json['method'] as String?,
      url: json['url'] as String?,
      statusCode: json['statusCode'] as int?,
      headers: _stringKeyMap(json['headers']),
      bodyText: json['bodyText'] as String? ?? '',
      meta: _stringKeyMap(json['meta']),
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

class ThreadDetail {
  const ThreadDetail({required this.thread, required this.messages});

  final ThreadSummary thread;
  final List<ThreadMessage> messages;

  String? get assistantId => thread.assistantId;

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

class Assistant {
  const Assistant({
    required this.id,
    required this.name,
    this.description = '',
    this.instructions = '',
    required this.version,
    required this.inferenceConnectionId,
    this.inferenceConnectionName,
    required this.defaultModel,
    this.settings = const {},
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final String description;
  final String instructions;
  final int version;
  final String? inferenceConnectionId;
  final String? inferenceConnectionName;
  final String? defaultModel;
  final Map<String, dynamic> settings;
  final DateTime createdAt;
  final DateTime updatedAt;

  bool get isComplete =>
      inferenceConnectionId != null &&
      inferenceConnectionId!.isNotEmpty &&
      defaultModel != null &&
      defaultModel!.isNotEmpty;

  factory Assistant.fromJson(Map<String, dynamic> json) {
    final settings = json['settings'];
    return Assistant(
      id: json['id'] as String,
      name: json['name'] as String,
      description: json['description'] as String? ?? '',
      instructions: json['instructions'] as String? ?? '',
      version: json['version'] as int? ?? 0,
      inferenceConnectionId: json['inferenceConnectionId'] as String?,
      inferenceConnectionName: json['inferenceConnectionName'] as String?,
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
    ExportMethod(id: 'netlify', label: 'Netlify', enabled: true),
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

class ExportLink {
  const ExportLink({required this.id, required this.label, required this.url});

  final String id;
  final String label;
  final String url;

  factory ExportLink.fromJson(Map<String, dynamic> json) {
    return ExportLink(
      id: json['id'] as String? ?? '',
      label: json['label'] as String? ?? json['id'] as String? ?? '',
      url: json['url'] as String? ?? '',
    );
  }
}

class ExportPublishResult {
  const ExportPublishResult({
    required this.method,
    required this.links,
    this.message,
  });

  final String method;
  final String? message;
  final List<ExportLink> links;

  factory ExportPublishResult.fromJson(Map<String, dynamic> json) {
    final rawLinks = json['links'];
    return ExportPublishResult(
      method: json['method'] as String? ?? '',
      message: json['message'] as String?,
      links: [
        if (rawLinks is List)
          for (final item in rawLinks)
            if (item is Map<String, dynamic>) ExportLink.fromJson(item),
      ],
    );
  }
}

/// Outcome of [CatalogClient.exportProject]: either a zip download or publish links.
sealed class ExportOutcome {
  const ExportOutcome();
}

final class ExportArchiveOutcome extends ExportOutcome {
  const ExportArchiveOutcome(this.archive);

  final ExportArchive archive;
}

final class ExportPublishOutcome extends ExportOutcome {
  const ExportPublishOutcome(this.result);

  final ExportPublishResult result;
}

class PlaneSettings {
  const PlaneSettings({
    this.sandbox = const {},
    this.environment = const {},
    this.integrations = const {},
    this.webSearchIntegrationId,
    this.fetchPageIntegrationId,
    this.platformInstructions = '',
    this.runtimeContext = '',
    this.permissions = const {},
  });

  /// Plane-wide permission rules, built-in tier overrides and gate scorers.
  final Map<String, dynamic> permissions;

  final Map<String, dynamic> sandbox;
  final Map<String, dynamic> environment;
  final Map<String, dynamic> integrations;
  final String? webSearchIntegrationId;
  final String? fetchPageIntegrationId;
  final String platformInstructions;
  final String runtimeContext;

  factory PlaneSettings.fromJson(Map<String, dynamic> json) {
    return PlaneSettings(
      sandbox: _stringKeyMap(json['sandbox']),
      environment: _stringKeyMap(json['environment']),
      integrations: _stringKeyMap(json['integrations']),
      webSearchIntegrationId: json['webSearchIntegrationId'] as String?,
      fetchPageIntegrationId: json['fetchPageIntegrationId'] as String?,
      platformInstructions: json['platformInstructions'] as String? ?? '',
      runtimeContext: json['runtimeContext'] as String? ?? '',
      permissions: _stringKeyMap(json['permissions']),
    );
  }
}

/// A catalog tool integration backing web_search or fetch_page.
class ToolIntegration {
  const ToolIntegration({
    required this.id,
    required this.name,
    required this.kind,
    required this.enabled,
    required this.scope,
    required this.endpoint,
    required this.mode,
    required this.capabilities,
    required this.secretsConfigured,
    required this.healthStatus,
  });

  final String id;
  final String name;
  final String kind;
  final bool enabled;
  final String scope;
  final String endpoint;
  final String mode;
  final List<String> capabilities;
  final Map<String, bool> secretsConfigured;
  final String healthStatus;

  bool get apiKeyConfigured => secretsConfigured['apiKey'] == true;

  factory ToolIntegration.fromJson(Map<String, dynamic> json) {
    final secrets = <String, bool>{};
    final rawSecrets = json['secretsConfigured'];
    if (rawSecrets is Map) {
      for (final e in rawSecrets.entries) {
        secrets['${e.key}'] = e.value == true;
      }
    }
    final caps = <String>[];
    final rawCaps = json['capabilities'];
    if (rawCaps is List) {
      for (final c in rawCaps) {
        caps.add('$c');
      }
    }
    return ToolIntegration(
      id: json['id'] as String,
      name: json['name'] as String? ?? '',
      kind: json['kind'] as String? ?? '',
      enabled: json['enabled'] as bool? ?? true,
      scope: json['scope'] as String? ?? 'plane',
      endpoint: json['endpoint'] as String? ?? '',
      mode: json['mode'] as String? ?? 'external',
      capabilities: caps,
      secretsConfigured: secrets,
      healthStatus: json['healthStatus'] as String? ?? 'unknown',
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
    co2Grams: _asDouble(part['co2Grams']),
    gpuEnergyJoules: _asDouble(part['gpuEnergyJoules']),
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
