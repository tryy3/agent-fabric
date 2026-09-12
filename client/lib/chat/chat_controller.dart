import 'dart:async';

import 'package:acpd/acpd.dart' show RpcError;
import 'package:flutter/foundation.dart';

import '../acp/agent_connection.dart';
import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import 'chat_message.dart';

enum ChatStatus { disconnected, connecting, connected, error }

/// Formats errors for the chat status line.
///
/// ACP maps handler failures to JSON-RPC `-32603` with message `"Internal error"`
/// and puts the real reason in [RpcError.data]; default [RpcError.toString] drops it.
@visibleForTesting
String formatChatError(Object error) {
  if (error is RpcError) {
    final data = error.data;
    if (data != null) {
      return 'RpcError(${error.code}): ${error.message}: $data';
    }
  }
  return error.toString();
}

class ChatController extends ChangeNotifier {
  ChatController({AgentSessionApi? session, CatalogClient? catalog})
      : _session = session ?? AgentConnection(),
        _catalog = catalog;

  final AgentSessionApi _session;
  final CatalogClient? _catalog;
  StreamSubscription<void>? _closedSub;

  ChatStatus status = ChatStatus.disconnected;
  String? statusMessage;
  final List<ChatMessage> messages = [];
  List<Agent> agents = [];
  String? selectedAgentId;
  bool _sending = false;
  bool _sessionReady = false;

  bool get canSend =>
      status == ChatStatus.connected && !_sending && _sessionReady;

  List<ModelOption> get modelOptions => _session.modelOptions;

  String? get currentModel => _session.currentModel;

  Future<void> connect() async {
    status = ChatStatus.connecting;
    statusMessage = null;
    _sessionReady = false;
    notifyListeners();
    await _closedSub?.cancel();
    _closedSub = null;
    try {
      await _session.connect();
      _closedSub = _session.closed.listen(_onSessionClosed);
      if (_catalog != null) {
        agents = await _catalog.listAgents();
      }
      status = ChatStatus.connected;
      statusMessage = null;
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  Future<void> selectAgent(String agentId) async {
    messages.clear();
    selectedAgentId = agentId;
    _sessionReady = false;
    notifyListeners();
    try {
      await _session.startSession(agentId);
      _sessionReady = true;
      if (status == ChatStatus.error) {
        status = ChatStatus.connected;
        statusMessage = null;
      }
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  Future<void> selectModel(String modelId) async {
    try {
      await _session.setModel(modelId);
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  void _onSessionClosed(void _) {
    if (_sending) return;
    if (status != ChatStatus.connected) return;
    status = ChatStatus.disconnected;
    _sessionReady = false;
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
      statusMessage = formatChatError(e);
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
