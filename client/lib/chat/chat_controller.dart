import 'dart:async';

import 'package:flutter/foundation.dart';

import '../acp/agent_connection.dart';
import 'chat_message.dart';

enum ChatStatus { disconnected, connecting, connected, error }

class ChatController extends ChangeNotifier {
  ChatController({AgentSessionApi? session})
      : _session = session ?? AgentConnection();

  final AgentSessionApi _session;
  StreamSubscription<void>? _closedSub;

  ChatStatus status = ChatStatus.disconnected;
  String? statusMessage;
  final List<ChatMessage> messages = [];
  bool _sending = false;

  bool get canSend =>
      status == ChatStatus.connected && !_sending;

  Future<void> connect() async {
    status = ChatStatus.connecting;
    statusMessage = null;
    notifyListeners();
    await _closedSub?.cancel();
    _closedSub = null;
    try {
      await _session.connect();
      _closedSub = _session.closed.listen(_onSessionClosed);
      status = ChatStatus.connected;
      statusMessage = null;
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = e.toString();
    }
    notifyListeners();
  }

  void _onSessionClosed(void _) {
    if (_sending) return;
    if (status != ChatStatus.connected) return;
    status = ChatStatus.disconnected;
    notifyListeners();
  }

  Future<void> send(String text) async {
    final trimmed = text.trim();
    if (!canSend || trimmed.isEmpty) return;

    messages.add(ChatMessage(role: ChatRole.user, text: trimmed));
    messages.add(const ChatMessage(role: ChatRole.assistant, text: ''));
    _sending = true;
    notifyListeners();

    try {
      await _session.sendPrompt(trimmed, onChunk: (chunk) {
        final last = messages.last;
        messages[messages.length - 1] = last.copyWith(text: last.text + chunk);
        notifyListeners();
      });
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = e.toString();
    } finally {
      _sending = false;
      notifyListeners();
    }
  }

  @override
  void dispose() {
    _closedSub?.cancel();
    _closedSub = null;
    _session.close();
    super.dispose();
  }
}
