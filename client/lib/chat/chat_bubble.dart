import '../acp/agent_connection.dart';
import '../catalog/models.dart';

enum ChatBubbleKind { user, thought, toolCall, message, stats }

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
    this.toolInput,
    this.toolOutput,
    this.streamingTool = false,
    this.createdAt,
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
  final Object? toolInput;
  final Object? toolOutput;
  final bool streamingTool;
  final DateTime? createdAt;

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
    Object? toolInput,
    Object? toolOutput,
    bool? streamingTool,
    DateTime? createdAt,
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
      toolInput: toolInput ?? this.toolInput,
      toolOutput: toolOutput ?? this.toolOutput,
      streamingTool: streamingTool ?? this.streamingTool,
      createdAt: createdAt ?? this.createdAt,
    );
  }
}

List<ChatBubble> bubblesFromThreadMessage(ThreadMessage message) {
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
  if (message.activities.isNotEmpty) {
    for (final activity in message.activities) {
      switch (activity) {
        case ThreadThoughtActivity(:final text):
          out.add(ChatBubble(kind: ChatBubbleKind.thought, text: text));
        case ThreadToolCallActivity(:final toolCall):
          out.add(
            ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: toolCall.id,
              toolTitle: toolCall.title,
              toolStatus: toolCall.status,
              toolInput: toolCall.input,
              toolOutput: toolCall.output,
              streamingTool:
                  toolCall.status != 'completed' && toolCall.status != 'failed',
            ),
          );
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
          streamingTool:
              toolCall.status != 'completed' && toolCall.status != 'failed',
        ),
      );
    }
  }
  out.add(
    ChatBubble(
      kind: ChatBubbleKind.message,
      text: message.content,
      model: message.model,
      providerName: message.providerName,
      predictedPerSecond: message.usage?.predictedPerSecond,
      createdAt: message.createdAt,
    ),
  );
  final stop = message.stopReason;
  if (message.usage != null || (stop != null && stop.isNotEmpty)) {
    out.add(
      ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: message.usage,
        stopReason: stop,
      ),
    );
  }
  return out;
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
  ].whereType<String>().where((part) => part.isNotEmpty).join(' · ');
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
