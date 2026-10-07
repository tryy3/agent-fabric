import '../acp/agent_connection.dart';
import '../acp/gate_info.dart';
import '../catalog/models.dart';

enum ChatBubbleKind {
  user,
  thought,
  sent,
  toolCall,
  message,
  stats,

  /// Cost and tokens of one LLM round, shown where its tool calls start.
  roundCost,
  requestFailed,
}

class ChatBubble {
  const ChatBubble({
    required this.kind,
    this.text = '',
    this.model,
    this.providerName,
    this.predictedPerSecond,
    this.usage,
    this.stopReason,
    this.streamingThought = false,
    this.toolCallId,
    this.toolTitle,
    this.toolStatus,
    this.toolKind,
    this.toolInput,
    this.toolOutput,
    this.toolGate,
    this.streamingTool = false,
    this.createdAt,
    this.catalogMessageId,
  });

  final ChatBubbleKind kind;
  final String text;
  final String? model;
  final String? providerName;
  final double? predictedPerSecond;
  final TurnUsage? usage;
  final String? stopReason;
  final bool streamingThought;
  final String? toolCallId;
  final String? toolTitle;
  final String? toolStatus;
  final String? toolKind;
  final Object? toolInput;
  final Object? toolOutput;
  final GateInfo? toolGate;
  final bool streamingTool;
  final DateTime? createdAt;

  /// Catalog message id when hydrated from thread history (assistant turns).
  final String? catalogMessageId;

  ChatBubble copyWith({
    String? text,
    String? model,
    String? providerName,
    double? predictedPerSecond,
    TurnUsage? usage,
    String? stopReason,
    bool? streamingThought,
    String? toolCallId,
    String? toolTitle,
    String? toolStatus,
    String? toolKind,
    Object? toolInput,
    Object? toolOutput,
    GateInfo? toolGate,
    bool? streamingTool,
    DateTime? createdAt,
    String? catalogMessageId,
  }) {
    return ChatBubble(
      kind: kind,
      text: text ?? this.text,
      model: model ?? this.model,
      providerName: providerName ?? this.providerName,
      predictedPerSecond: predictedPerSecond ?? this.predictedPerSecond,
      usage: usage ?? this.usage,
      stopReason: stopReason ?? this.stopReason,
      streamingThought: streamingThought ?? this.streamingThought,
      toolCallId: toolCallId ?? this.toolCallId,
      toolTitle: toolTitle ?? this.toolTitle,
      toolStatus: toolStatus ?? this.toolStatus,
      toolKind: toolKind ?? this.toolKind,
      toolInput: toolInput ?? this.toolInput,
      toolOutput: toolOutput ?? this.toolOutput,
      toolGate: toolGate ?? this.toolGate,
      streamingTool: streamingTool ?? this.streamingTool,
      createdAt: createdAt ?? this.createdAt,
      catalogMessageId: catalogMessageId ?? this.catalogMessageId,
    );
  }
}

/// The divider for one LLM round of a tool-using turn, from the rounds the
/// plane stored beside the message. Null when there is no data for [round].
ChatBubble? roundCostBubble(TurnUsage? usage, int round, {String? messageId}) {
  if (usage == null) return null;
  for (final r in usage.rounds) {
    if (r['round'] != round) continue;
    // A stored round keeps its own cost under "cost"; a live round update
    // keeps it under "roundCost" (its "cost" is the running turn total).
    final meta =
        <String, Object?>{...r, 'partial': true, 'roundCost': r['cost']}
          ..remove('cost')
          ..remove('partIndex');
    return ChatBubble(
      kind: ChatBubbleKind.roundCost,
      usage: turnUsageFromMeta(meta),
      catalogMessageId: messageId,
    );
  }
  return null;
}

/// Adds a round divider where each round begins in a message's bubbles. Only
/// every turn gets one per round, even a single-round turn.
class _RoundDividers {
  _RoundDividers(this._message, this._out)
    : _starts = [..._message.roundStarts]
        ..sort((a, b) => a.round.compareTo(b.round));

  final ThreadMessage _message;
  final List<ChatBubble> _out;
  final List<RoundStart> _starts;
  int _next = 0;

  /// Adds the dividers of every round that begins at or before [partIndex].
  void before(int partIndex) {
    while (_next < _starts.length && _starts[_next].partIndex <= partIndex) {
      final divider = roundCostBubble(
        _message.usage,
        _starts[_next].round,
        messageId: _message.id,
      );
      if (divider != null) _out.add(divider);
      _next++;
    }
  }

  /// Adds the dividers of the rounds not placed yet (the answering round
  /// begins at the message, which is not an activity).
  void rest() => before(1 << 30);
}

List<ChatBubble> bubblesFromThreadMessages(List<ThreadMessage> messages) {
  final assistantsByPrompt = <String, List<ThreadMessage>>{};
  for (final message in messages) {
    if (message.role != 'assistant') {
      continue;
    }
    final promptId = message.promptMessageId;
    if (promptId == null) {
      continue;
    }
    (assistantsByPrompt[promptId] ??= []).add(message);
  }
  for (final attempts in assistantsByPrompt.values) {
    attempts.sort((a, b) => a.position.compareTo(b.position));
  }

  final out = <ChatBubble>[];
  final handledPrompts = <String>{};
  String? promptText;
  for (final message in messages) {
    for (final activity in message.activities) {
      if (activity case TurnSentActivity(:final text) when text.isNotEmpty) {
        promptText ??= text;
      }
    }
  }
  if (promptText != null) {
    out.add(ChatBubble(kind: ChatBubbleKind.sent, text: promptText));
  }
  for (final message in messages) {
    if (message.role == 'user') {
      out.addAll(bubblesFromThreadMessage(message));
      handledPrompts.add(message.id);
      final tip = _displayAttemptForPrompt(
        assistantsByPrompt[message.id] ?? const [],
      );
      if (tip != null) {
        out.addAll(_bubblesForDisplayAttempt(tip));
      }
      continue;
    }
    if (message.role != 'assistant') {
      continue;
    }
    final promptId = message.promptMessageId;
    if (promptId != null) {
      if (handledPrompts.contains(promptId)) {
        continue;
      }
      handledPrompts.add(promptId);
      final tip = _displayAttemptForPrompt(
        assistantsByPrompt[promptId] ?? const [],
      );
      if (tip != null) {
        out.addAll(_bubblesForDisplayAttempt(tip));
      }
      continue;
    }
    out.addAll(bubblesFromThreadMessage(message));
  }
  return out;
}

/// Soft-supersede fork tip for one user prompt: a newer failed draft wins over
/// an older active prior; a successful active attempt hides earlier failures.
ThreadMessage? _displayAttemptForPrompt(List<ThreadMessage> attempts) {
  if (attempts.isEmpty) {
    return null;
  }
  final latest = attempts.last;
  if (latest.status == 'failed') {
    return latest;
  }
  for (var i = attempts.length - 1; i >= 0; i--) {
    if (attempts[i].active) {
      return attempts[i];
    }
  }
  return null;
}

List<ChatBubble> _bubblesForDisplayAttempt(ThreadMessage tip) {
  if (tip.status == 'failed') {
    // Active or inactive: keep every completed part up to the failure, then the
    // error row. Inactive is the soft-supersede retry draft tip.
    return _bubblesFromFailedAttempt(tip);
  }
  return bubblesFromThreadMessage(tip);
}

/// Renders a failed attempt including partial thought/tool/message content.
List<ChatBubble> _bubblesFromFailedAttempt(ThreadMessage message) {
  final out = <ChatBubble>[];
  final dividers = _RoundDividers(message, out);
  final errors = <String>[];
  if (message.activities.isNotEmpty) {
    for (var i = 0; i < message.activities.length; i++) {
      final activity = message.activities[i];
      if (i < message.activityPartIndexes.length) {
        dividers.before(message.activityPartIndexes[i]);
      }
      switch (activity) {
        case TurnThoughtActivity(:final text):
          out.add(ChatBubble(kind: ChatBubbleKind.thought, text: text));
        case TurnSentActivity():
          // Hoisted to the top of the thread in [bubblesFromThreadMessages].
          break;
        case TurnToolCallActivity(:final toolCall):
          out.add(
            ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: toolCall.id,
              toolTitle: toolCall.title,
              toolStatus: toolCall.status,
              toolInput: toolCall.input,
              toolOutput: toolCall.output,
              toolGate: toolCall.gate,
              streamingTool: false,
            ),
          );
        case TurnErrorActivity(:final text):
          if (text.isNotEmpty) {
            errors.add(text);
          }
      }
    }
  } else {
    final thought = message.thought;
    if (thought != null && thought.isNotEmpty) {
      out.add(ChatBubble(kind: ChatBubbleKind.thought, text: thought));
    }
    for (final toolCall in message.toolCalls) {
      out.add(
        ChatBubble(
          kind: ChatBubbleKind.toolCall,
          toolCallId: toolCall.id,
          toolTitle: toolCall.title,
          toolStatus: toolCall.status,
          toolInput: toolCall.input,
          toolOutput: toolCall.output,
          toolGate: toolCall.gate,
          streamingTool: false,
        ),
      );
    }
  }
  if (message.content.isNotEmpty) {
    dividers.rest();
    out.add(
      ChatBubble(
        kind: ChatBubbleKind.message,
        text: message.content,
        model: message.model,
        providerName: message.providerName,
        predictedPerSecond: message.usage?.predictedPerSecond,
        createdAt: message.createdAt,
        catalogMessageId: message.id,
      ),
    );
  }
  if (errors.isEmpty) {
    errors.addAll(_synthesizedFailureTexts(message));
  }
  for (final text in errors) {
    out.add(ChatBubble(kind: ChatBubbleKind.requestFailed, text: text));
  }
  final stop = message.stopReason;
  if (message.usage != null || (stop != null && stop.isNotEmpty)) {
    out.add(
      ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: message.usage,
        stopReason: stop,
        catalogMessageId: message.id,
      ),
    );
  }
  return out;
}

List<ChatBubble> bubblesFromThreadMessage(ThreadMessage message) {
  if (!message.active) {
    return const [];
  }
  if (message.role == 'user') {
    return [
      ChatBubble(
        kind: ChatBubbleKind.user,
        text: message.content,
        createdAt: message.createdAt,
      ),
    ];
  }
  final out = <ChatBubble>[];
  final dividers = _RoundDividers(message, out);
  final errors = <String>[];
  if (message.activities.isNotEmpty) {
    for (var i = 0; i < message.activities.length; i++) {
      final activity = message.activities[i];
      if (i < message.activityPartIndexes.length) {
        dividers.before(message.activityPartIndexes[i]);
      }
      switch (activity) {
        case TurnThoughtActivity(:final text):
          out.add(ChatBubble(kind: ChatBubbleKind.thought, text: text));
        case TurnSentActivity():
          // Hoisted to the top of the thread in [bubblesFromThreadMessages].
          break;
        case TurnToolCallActivity(:final toolCall):
          out.add(
            ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: toolCall.id,
              toolTitle: toolCall.title,
              toolStatus: toolCall.status,
              toolInput: toolCall.input,
              toolOutput: toolCall.output,
              toolGate: toolCall.gate,
              streamingTool:
                  toolCall.status != 'completed' && toolCall.status != 'failed',
            ),
          );
        case TurnErrorActivity(:final text):
          if (text.isNotEmpty) {
            errors.add(text);
          }
      }
    }
  } else {
    final thought = message.thought;
    if (thought != null && thought.isNotEmpty) {
      out.add(ChatBubble(kind: ChatBubbleKind.thought, text: thought));
    }
    for (final toolCall in message.toolCalls) {
      out.add(
        ChatBubble(
          kind: ChatBubbleKind.toolCall,
          toolCallId: toolCall.id,
          toolTitle: toolCall.title,
          toolStatus: toolCall.status,
          toolInput: toolCall.input,
          toolOutput: toolCall.output,
          toolGate: toolCall.gate,
          streamingTool:
              toolCall.status != 'completed' && toolCall.status != 'failed',
        ),
      );
    }
  }
  dividers.rest();
  out.add(
    ChatBubble(
      kind: ChatBubbleKind.message,
      text: message.content,
      model: message.model,
      providerName: message.providerName,
      predictedPerSecond: message.usage?.predictedPerSecond,
      createdAt: message.createdAt,
      catalogMessageId: message.id,
    ),
  );
  if (errors.isEmpty && message.status == 'failed') {
    errors.addAll(_synthesizedFailureTexts(message));
  }
  for (final text in errors) {
    out.add(ChatBubble(kind: ChatBubbleKind.requestFailed, text: text));
  }
  final stop = message.stopReason;
  if (message.usage != null || (stop != null && stop.isNotEmpty)) {
    out.add(
      ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: message.usage,
        stopReason: stop,
        catalogMessageId: message.id,
      ),
    );
  }
  return out;
}

List<String> _synthesizedFailureTexts(ThreadMessage message) {
  final stop = message.stopReason;
  return [
    if (stop == null || stop.isEmpty)
      'Inference failed'
    else
      'Inference failed: $stop',
  ];
}

String bubbleCaption(ChatBubble bubble) {
  if (bubble.kind != ChatBubbleKind.message) {
    return '';
  }
  final tok = bubble.predictedPerSecond;
  return [
    bubble.model,
    bubble.providerName,
    if (tok != null) '${_captionToks(tok)} tok/s',
  ].whereType<String>().where((part) => part.isNotEmpty).join(' - ');
}

String _captionToks(double tok) {
  return tok.toStringAsFixed(2).replaceFirst(RegExp(r'\.?0+$'), '');
}

String activityDescription(String text) {
  for (final line in text.split('\n')) {
    final trimmed = line.trim();
    if (trimmed.isNotEmpty) {
      return trimmed;
    }
  }
  return '';
}
