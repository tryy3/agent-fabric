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

String _autoTitle(String prompt) {
  final fields = prompt
      .trim()
      .split(RegExp(r'\s+'))
      .where((w) => w.isNotEmpty)
      .toList();
  if (fields.isEmpty) {
    return 'Untitled';
  }
  return fields.take(8).join(' ');
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
  List<ThreadSummary> threads = [];
  String threadFilter = '';
  String? selectedThreadId;
  String? selectedAgentId;
  bool _sending = false;
  bool _sessionReady = false;
  bool _sessionStarting = false;
  int _sendEpoch = 0;
  int _threadLoadEpoch = 0;
  int _uncommittedStart = 0;

  ThreadSummary? get selectedThread {
    final id = selectedThreadId;
    if (id == null) {
      return null;
    }
    for (final t in threads) {
      if (t.id == id) {
        return t;
      }
    }
    return null;
  }

  List<ThreadSummary> get visibleThreads {
    final q = threadFilter.trim().toLowerCase();
    if (q.isEmpty) {
      return threads;
    }
    return threads.where((t) => t.title.toLowerCase().contains(q)).toList();
  }

  bool get selectedAgentMissing {
    final id = selectedAgentId;
    if (id == null) {
      return false;
    }
    return !agents.any((a) => a.id == id);
  }

  bool get selectedAgentIsComplete {
    final id = selectedAgentId;
    if (id == null) {
      return false;
    }
    for (final a in agents) {
      if (a.id == id) {
        return a.isComplete;
      }
    }
    return false;
  }

  bool get canSend =>
      status == ChatStatus.connected &&
      !_sending &&
      _sessionReady &&
      selectedThreadId != null &&
      selectedThread?.agentId != null &&
      selectedAgentIsComplete;

  bool get canSelectAgent =>
      status == ChatStatus.connected &&
      !_sessionStarting &&
      selectedThreadId != null &&
      selectedThread?.agentId == null;

  bool get canSelectModel =>
      status == ChatStatus.connected && !_sessionStarting && _sessionReady;

  List<ModelOption> get modelOptions => _session.modelOptions;

  String? get currentModel => _session.currentModel;

  Future<void> connect() async {
    if (status == ChatStatus.connected || status == ChatStatus.connecting) {
      return;
    }
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
        threads = await _catalog.listThreads();
        status = ChatStatus.connected;
        statusMessage = null;
        notifyListeners();
        if (threads.isNotEmpty) {
          await selectThread(threads.first.id);
          return;
        }
      }
      status = ChatStatus.connected;
      statusMessage = null;
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  Future<void> createThread() async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      final created = await catalog.createThread();
      threads.insert(0, created);
      notifyListeners();
      await selectThread(created.id);
    } catch (e) {
      statusMessage = formatChatError(e);
      notifyListeners();
    }
  }

  Future<void> reloadAgents() async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      agents = await catalog.listAgents();
    } catch (e) {
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    if (_sessionStarting) {
      notifyListeners();
      return;
    }
    final id = selectedAgentId;
    if (id != null && selectedAgentIsComplete && !_sessionReady) {
      await _startSession(id, rebind: true);
      return;
    }
    if (!selectedAgentIsComplete) {
      _sessionReady = false;
    }
    notifyListeners();
  }

  void setThreadFilter(String query) {
    threadFilter = query;
    notifyListeners();
  }

  Future<void> renameThread(String id, String title) async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      final updated = await catalog.renameThread(id, title);
      _replaceThread(updated);
    } catch (e) {
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  Future<void> selectThread(String id) async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    if (_sending) {
      try {
        await _session.cancel();
      } catch (e) {
        statusMessage = formatChatError(e);
        notifyListeners();
        return;
      }
      _sendEpoch++;
      _sending = false;
      _dropUncommitted();
    }
    _threadLoadEpoch++;
    final ThreadDetail detail;
    try {
      detail = await catalog.getThread(id);
    } on CatalogException catch (e) {
      if (e.statusCode == 404) {
        selectedThreadId = null;
        messages.clear();
        selectedAgentId = null;
        _sessionReady = false;
        try {
          threads = await catalog.listThreads();
        } catch (listErr) {
          statusMessage = formatChatError(listErr);
          notifyListeners();
          return;
        }
        statusMessage = formatChatError(e);
        notifyListeners();
        return;
      }
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    } catch (e) {
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    selectedThreadId = id;
    _replaceThread(detail.thread);
    messages
      ..clear()
      ..addAll(detail.messages.map(_chatMessageFromThread));
    final agentId = detail.thread.agentId;
    if (agentId != null) {
      selectedAgentId = agentId;
      if (!selectedAgentIsComplete) {
        _sessionReady = false;
        notifyListeners();
        return;
      }
      _sessionStarting = true;
      _sessionReady = false;
      notifyListeners();
      try {
        await _session.startSession(agentId, threadId: id);
        selectedAgentId = agentId;
        _sessionReady = true;
        status = ChatStatus.connected;
        statusMessage = null;
      } catch (e) {
        selectedAgentId = null;
        _sessionReady = false;
        statusMessage = formatChatError(e);
      } finally {
        _sessionStarting = false;
        notifyListeners();
      }
      return;
    }
    selectedAgentId = null;
    _sessionReady = false;
    notifyListeners();
  }

  Future<void> selectAgent(String agentId) async {
    final match = agents.where((a) => a.id == agentId);
    if (match.isEmpty || !match.first.isComplete) {
      return;
    }
    await _startSession(agentId, rebind: false);
  }

  Future<void> _startSession(String agentId, {required bool rebind}) async {
    final threadId = selectedThreadId;
    if (threadId == null) {
      return;
    }
    if (!rebind && selectedThread?.agentId != null) {
      return;
    }
    final previousReady = _sessionReady;
    _sessionStarting = true;
    _sessionReady = false;
    notifyListeners();
    try {
      await _session.startSession(agentId, threadId: threadId);
      selectedAgentId = agentId;
      _pinSelectedAgent(agentId);
      _sessionReady = true;
      status = ChatStatus.connected;
      statusMessage = null;
    } catch (e) {
      _sessionReady = previousReady;
      statusMessage = formatChatError(e);
    } finally {
      _sessionStarting = false;
      notifyListeners();
    }
  }

  Future<void> selectModel(String modelId) async {
    try {
      await _session.setModel(modelId);
    } catch (e) {
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

    final epoch = ++_sendEpoch;
    _uncommittedStart = messages.length;
    messages.add(ChatMessage(role: ChatRole.user, text: trimmed));
    messages.add(
      ChatMessage(
        role: ChatRole.assistant,
        text: '',
        model: currentModel,
        providerName: _selectedProviderName(),
      ),
    );
    _sending = true;
    notifyListeners();

    try {
      await _session.sendPrompt(trimmed, onEvent: (event) {
        if (epoch != _sendEpoch || messages.isEmpty) {
          return;
        }
        final last = messages.last;
        switch (event) {
          case AgentThoughtDelta(:final text):
            messages[messages.length - 1] = last.copyWith(
              thought: '${last.thought ?? ''}$text',
              streamingThought: true,
            );
          case AgentMessageDelta(:final text):
            messages[messages.length - 1] = last.copyWith(
              text: last.text + text,
            );
          case AgentUsageEvent(:final usage):
            messages[messages.length - 1] = last.copyWith(
              usage: usage,
              stopReason: usage.stopReason,
            );
        }
        notifyListeners();
      });
      if (epoch != _sendEpoch) {
        return;
      }
      if (messages.isNotEmpty && messages.last.streamingThought) {
        messages[messages.length - 1] = messages.last.copyWith(
          streamingThought: false,
        );
      }
      _sending = false;
      try {
        await _refreshSelectedThread(
          optimisticTitle: _autoTitle(trimmed),
          epoch: epoch,
        );
      } catch (e) {
        statusMessage = formatChatError(e);
      }
    } catch (e) {
      if (epoch != _sendEpoch) {
        return;
      }
      _dropUncommitted();
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    } finally {
      if (epoch == _sendEpoch) {
        _sending = false;
      }
      notifyListeners();
    }
  }

  Future<void> _refreshSelectedThread({
    required String optimisticTitle,
    int? epoch,
  }) async {
    final catalog = _catalog;
    final id = selectedThreadId;
    if (catalog == null || id == null) {
      return;
    }
    final loadGen = _threadLoadEpoch;
    final detail = await catalog.getThread(id);
    if (selectedThreadId != id ||
        loadGen != _threadLoadEpoch ||
        (epoch != null && epoch != _sendEpoch)) {
      return;
    }
    final local = selectedThread;
    var summary = detail.thread;
    if (summary.agentId == null && local?.agentId != null) {
      summary = _copyThread(summary, agentId: local!.agentId);
    }
    if (summary.titleSource == 'auto' &&
        (summary.title == 'Untitled' || summary.title.isEmpty)) {
      if (local?.titleSource == 'user') {
        summary = _copyThread(
          summary,
          title: local!.title,
          titleSource: 'user',
        );
      } else if (local != null &&
          local.title.isNotEmpty &&
          local.title != 'Untitled') {
        summary = _copyThread(summary, title: local.title);
      } else {
        summary = _copyThread(summary, title: optimisticTitle);
      }
    }
    _replaceThread(summary, promote: true);
    if (detail.messages.isNotEmpty) {
      messages
        ..clear()
        ..addAll(detail.messages.map(_chatMessageFromThread));
    }
  }

  void _pinSelectedAgent(String agentId) {
    final current = selectedThread;
    if (current == null) {
      return;
    }
    _replaceThread(_copyThread(current, agentId: agentId));
  }

  void _replaceThread(ThreadSummary thread, {bool promote = false}) {
    final i = threads.indexWhere((t) => t.id == thread.id);
    if (i >= 0) {
      threads.removeAt(i);
    }
    if (promote || i < 0) {
      threads.insert(0, thread);
    } else {
      threads.insert(i, thread);
    }
  }

  ThreadSummary _copyThread(
    ThreadSummary t, {
    String? title,
    String? titleSource,
    String? agentId,
    DateTime? updatedAt,
  }) {
    return ThreadSummary(
      id: t.id,
      title: title ?? t.title,
      titleSource: titleSource ?? t.titleSource,
      agentId: agentId ?? t.agentId,
      currentModel: t.currentModel,
      messageCount: t.messageCount,
      createdAt: t.createdAt,
      updatedAt: updatedAt ?? t.updatedAt,
    );
  }

  String? _selectedProviderName() {
    final id = selectedAgentId;
    if (id == null) {
      return null;
    }
    for (final a in agents) {
      if (a.id == id) {
        return a.providerName;
      }
    }
    return null;
  }

  ChatMessage _chatMessageFromThread(ThreadMessage m) {
    return ChatMessage(
      role: m.role == 'user' ? ChatRole.user : ChatRole.assistant,
      text: m.content,
      thought: m.thought,
      model: m.model,
      providerName: m.providerName,
      usage: m.usage,
      stopReason: m.stopReason,
    );
  }

  void _dropUncommitted() {
    if (messages.length > _uncommittedStart) {
      messages.removeRange(_uncommittedStart, messages.length);
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
