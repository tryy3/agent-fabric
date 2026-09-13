import '../acp/agent_connection.dart';

enum ChatRole { user, assistant }

class ChatMessage {
  const ChatMessage({
    required this.role,
    required this.text,
    this.thought,
    this.model,
    this.providerName,
    this.usage,
    this.stopReason,
    this.streamingThought = false,
  });

  final ChatRole role;
  final String text;
  final String? thought;
  final String? model;
  final String? providerName;
  final TurnUsage? usage;
  final String? stopReason;
  final bool streamingThought;

  ChatMessage copyWith({
    String? text,
    String? thought,
    String? model,
    String? providerName,
    TurnUsage? usage,
    String? stopReason,
    bool? streamingThought,
  }) => ChatMessage(
    role: role,
    text: text ?? this.text,
    thought: thought ?? this.thought,
    model: model ?? this.model,
    providerName: providerName ?? this.providerName,
    usage: usage ?? this.usage,
    stopReason: stopReason ?? this.stopReason,
    streamingThought: streamingThought ?? this.streamingThought,
  );
}
