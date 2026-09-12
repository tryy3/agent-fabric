enum ChatRole { user, assistant }

class ChatMessage {
  const ChatMessage({required this.role, required this.text});

  final ChatRole role;
  final String text;

  ChatMessage copyWith({String? text}) =>
      ChatMessage(role: role, text: text ?? this.text);
}
