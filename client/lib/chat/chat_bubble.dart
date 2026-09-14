import '../acp/agent_connection.dart';
import '../catalog/models.dart';

enum ChatBubbleKind { user, thought, message, stats }

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
  });

  final ChatBubbleKind kind;
  final String text;
  final String? model;
  final String? providerName;
  final double? predictedPerSecond;
  final TurnUsage? usage;
  final String? stopReason;
  final bool streamingThought;

  ChatBubble copyWith({
    String? text,
    String? model,
    String? providerName,
    double? predictedPerSecond,
    TurnUsage? usage,
    String? stopReason,
    bool? streamingThought,
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
    );
  }
}

List<ChatBubble> bubblesFromThreadMessage(ThreadMessage message) {
  if (message.role == 'user') {
    return [ChatBubble(kind: ChatBubbleKind.user, text: message.content)];
  }
  final out = <ChatBubble>[];
  final thought = message.thought;
  if (thought != null && thought.isNotEmpty) {
    out.add(ChatBubble(kind: ChatBubbleKind.thought, text: thought));
  }
  out.add(
    ChatBubble(
      kind: ChatBubbleKind.message,
      text: message.content,
      model: message.model,
      providerName: message.providerName,
      predictedPerSecond: message.usage?.predictedPerSecond,
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
