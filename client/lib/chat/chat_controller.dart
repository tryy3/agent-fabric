import 'dart:async';

import 'package:acpd/acpd.dart' hide AgentConnection;
import 'package:flutter/foundation.dart';

import '../acp/agent_connection.dart';
import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../catalog/save_export.dart';
import 'ask_user_question.dart';
import 'chat_bubble.dart';
import 'pending_interaction.dart';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';

enum ChatStatus { disconnected, connecting, connected, reconnecting, error }

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
    return 'RpcError(${error.code}): ${error.message}';
  }
  if (error is CatalogException) {
    // Server message is already operator-facing; never dump CatalogException(...).
    if (error.message.isNotEmpty) {
      return error.message;
    }
    return operatorMessageFor(
      CatalogRequestFailure(statusCode: error.statusCode),
    );
  }
  final raw = '$error';
  if (raw.isEmpty || raw.startsWith('Instance of ')) {
    return operatorMessageFromError(error);
  }
  return raw;
}

void _logCatch(String where, Object error, StackTrace stack) {
  AppLog.record('$where: $error', stack);
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
  ChatController({
    AgentSessionApi? session,
    this._catalog,
    SaveExportBytes? saveExport,
  }) : _session = session ?? AgentConnection(),
       _saveExport = saveExport ?? saveExportBytes;

  final AgentSessionApi _session;
  final CatalogClient? _catalog;
  final SaveExportBytes _saveExport;
  StreamSubscription<AcpConnectionState>? _stateSub;

  /// Underlying ACP session when it is a real [AgentConnection].
  AgentConnection? get agentConnection =>
      _session is AgentConnection ? _session : null;

  ChatStatus status = ChatStatus.disconnected;
  String? statusMessage;
  final List<ChatBubble> messages = [];
  List<Agent> agents = [];
  List<Provider> providers = [];
  List<Project> projects = [];
  List<ThreadSummary> threads = [];

  /// Threads loaded for projects other than the selected one.
  final Map<String, List<ThreadSummary>> projectThreads = {};

  /// Last-thread lookup used by [connect] and [selectProject].
  Future<String?> Function(String projectId)? preferredThread;

  /// Last-project lookup used by [connect] to restore the active workspace.
  Future<String?> Function()? preferredProject;
  String threadFilter = '';
  String? selectedThreadId;
  String? selectedProjectId;
  List<ExportMethod> exporters = List.of(ExportMethod.defaults);
  String? selectedAgentId;
  VoidCallback? onAgentTurnCommitted;
  bool _sending = false;
  bool get sending => _sending;
  bool _sessionReady = false;
  bool _restoreSessionReadyOnConnect = false;
  bool _sessionStarting = false;
  int _sendEpoch = 0;
  int _threadLoadEpoch = 0;
  int _uncommittedStart = 0;

  /// Thread that owns the live ACP session (may differ from [selectedThreadId]).
  String? _sessionOwnerThreadId;

  /// Mid-turn permission / ask_user waits keyed by catalog thread id.
  final Map<String, PendingInteraction> _pendingByThread = {};

  /// Live turn transcript parked while the user browses another thread.
  List<ChatBubble>? _parkedLiveMessages;
  String? _parkedLiveThreadId;

  /// Pending interaction for the currently selected thread, if any.
  PendingInteraction? get selectedPending {
    final id = selectedThreadId;
    if (id == null) return null;
    return _pendingByThread[id];
  }

  /// True when another thread still has an unresolved permission/ask_user wait.
  bool get waitingOnOtherThread {
    final id = selectedThreadId;
    if (_pendingByThread.isEmpty) return false;
    if (id == null) return true;
    return !_pendingByThread.containsKey(id) && _pendingByThread.isNotEmpty;
  }

  bool get _ownerTurnLive {
    final owner = _sessionOwnerThreadId;
    if (owner == null) return false;
    return _sending || _pendingByThread.containsKey(owner);
  }

  /// Bubbles for the live ACP turn (parked while viewing another thread).
  List<ChatBubble> get _liveMessages {
    final parked = _parkedLiveMessages;
    if (parked != null && _parkedLiveThreadId != null) {
      return parked;
    }
    return messages;
  }

  void _parkLiveTranscriptIfNeeded() {
    final owner = _sessionOwnerThreadId;
    if (owner == null || !_ownerTurnLive) {
      return;
    }
    if (selectedThreadId != owner) {
      return;
    }
    if (_parkedLiveThreadId == owner && _parkedLiveMessages != null) {
      return;
    }
    _parkedLiveMessages = List<ChatBubble>.of(messages);
    _parkedLiveThreadId = owner;
  }

  void _unparkLiveTranscriptIntoMessages() {
    final parked = _parkedLiveMessages;
    if (parked == null || _parkedLiveThreadId == null) {
      return;
    }
    messages
      ..clear()
      ..addAll(parked);
    _parkedLiveMessages = null;
    _parkedLiveThreadId = null;
  }

  void _clearParkedLiveTranscript() {
    _parkedLiveMessages = null;
    _parkedLiveThreadId = null;
  }

  /// Injects a pending interaction for tests without a live [AgentConnection].
  @visibleForTesting
  void putPendingForTest(PendingInteraction pending) {
    _pendingByThread[pending.threadId] = pending;
    _sessionOwnerThreadId ??= pending.threadId;
    notifyListeners();
  }

  void _bindInteractionHandlers() {
    final conn = agentConnection;
    if (conn == null) {
      return;
    }
    conn.permissionHandler = (request, cancellation) async {
      final threadId = _sessionOwnerThreadId ?? selectedThreadId;
      if (threadId == null) {
        return const RequestPermissionResponse(outcome: PermissionCancelled());
      }
      final completer = Completer<RequestPermissionResponse>();
      final pending = PendingPermission(
        threadId: threadId,
        request: request,
        completer: completer,
      );
      _pendingByThread[threadId] = pending;
      notifyListeners();
      unawaited(
        cancellation.whenCancelled.then((_) {
          _completePending(threadId, cancelledPermission: true);
        }),
      );
      return completer.future;
    };
    conn.elicitationHandler = (params, cancellation) async {
      final threadId = _sessionOwnerThreadId ?? selectedThreadId;
      if (threadId == null) {
        return <String, Object?>{'action': 'cancel'};
      }
      final questions = parseAskUserQuestions(params);
      if (questions.isEmpty) {
        return <String, Object?>{'action': 'decline'};
      }
      final completer = Completer<Map<String, Object?>>();
      final pending = PendingAskUser(
        threadId: threadId,
        message: '${params['message'] ?? ''}'.trim(),
        questions: questions,
        completer: completer,
      );
      _pendingByThread[threadId] = pending;
      notifyListeners();
      unawaited(
        cancellation.whenCancelled.then((_) {
          _completePending(threadId, cancelledAsk: true);
        }),
      );
      final content = await completer.future;
      if (content.containsKey('__cancelled__')) {
        return <String, Object?>{'action': 'cancel'};
      }
      if (content.containsKey('__skipped__')) {
        return <String, Object?>{'action': 'cancel'};
      }
      return <String, Object?>{'action': 'accept', 'content': content};
    };
  }

  void resolvePermission(String optionId) {
    final pending = selectedPending;
    if (pending is! PendingPermission || pending.completer.isCompleted) {
      return;
    }
    _pendingByThread.remove(pending.threadId);
    pending.completer.complete(
      RequestPermissionResponse(
        outcome: PermissionSelected(optionId: optionId),
      ),
    );
    notifyListeners();
  }

  void submitAskUser(Map<String, Object?> content) {
    final pending = selectedPending;
    if (pending is! PendingAskUser || pending.completer.isCompleted) {
      return;
    }
    _pendingByThread.remove(pending.threadId);
    pending.completer.complete(content);
    notifyListeners();
  }

  void skipAskUser() {
    final pending = selectedPending;
    if (pending is! PendingAskUser || pending.completer.isCompleted) {
      return;
    }
    _pendingByThread.remove(pending.threadId);
    pending.completer.complete({'__skipped__': true});
    notifyListeners();
  }

  void _completePending(
    String threadId, {
    bool cancelledPermission = false,
    bool cancelledAsk = false,
  }) {
    final pending = _pendingByThread.remove(threadId);
    if (pending == null) {
      return;
    }
    switch (pending) {
      case PendingPermission(:final completer):
        if (!completer.isCompleted && cancelledPermission) {
          completer.complete(
            const RequestPermissionResponse(outcome: PermissionCancelled()),
          );
        }
      case PendingAskUser(:final completer):
        if (!completer.isCompleted && cancelledAsk) {
          completer.complete({'__cancelled__': true});
        }
    }
    notifyListeners();
  }

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

  Project? get selectedProject {
    final id = selectedProjectId;
    if (id == null) {
      return null;
    }
    for (final p in projects) {
      if (p.id == id) {
        return p;
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
      selectedAgentIsComplete &&
      selectedPending == null &&
      !waitingOnOtherThread;

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
    if (status == ChatStatus.connected ||
        status == ChatStatus.connecting ||
        status == ChatStatus.reconnecting) {
      return;
    }
    status = ChatStatus.connecting;
    statusMessage = null;
    _sessionReady = false;
    notifyListeners();
    await _stateSub?.cancel();
    _stateSub = null;
    try {
      await _session.connect();
      _bindInteractionHandlers();
      _stateSub = _session.connectionState.listen(_onConnectionState);
      if (_catalog != null) {
        agents = await _catalog.listAgents();
        projects = await _catalog.listProjects();
        selectedProjectId = await _restoreInitialProjectId();
        _publishThreads(
          await _catalog.listThreads(projectId: selectedProjectId),
        );
        providers = await _catalog.listProviders();
        await _refreshExporters();
        status = ChatStatus.connected;
        statusMessage = null;
        notifyListeners();
        final opening = await _preferredThreadId(selectedProjectId);
        if (opening != null) {
          await selectThread(opening);
          return;
        }
      }
      status = ChatStatus.connected;
      statusMessage = null;
    } on Object catch (e, s) {
      _logCatch('connect', e, s);
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  void _onConnectionState(AcpConnectionState state) {
    switch (state) {
      case AcpConnectionState.connecting:
        status = ChatStatus.connecting;
      case AcpConnectionState.reconnecting:
        status = ChatStatus.reconnecting;
        _restoreSessionReadyOnConnect =
            _restoreSessionReadyOnConnect || _sessionReady;
        _sessionReady = false;
      case AcpConnectionState.connected:
        status = ChatStatus.connected;
        if (_restoreSessionReadyOnConnect) {
          _sessionReady = true;
          _restoreSessionReadyOnConnect = false;
        }
        statusMessage = null;
        unawaited(
          _refreshCatalogAfterReconnect().catchError((Object e, StackTrace s) {
            _logCatch('reconnect catalog', e, s);
          }),
        );
      case AcpConnectionState.disconnected:
        status = ChatStatus.disconnected;
        _sessionReady = false;
        _restoreSessionReadyOnConnect = false;
    }
    notifyListeners();
  }

  Future<void> _refreshCatalogAfterReconnect() async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    Object? refreshError;
    try {
      agents = await catalog.listAgents();
    } on Object catch (e, s) {
      _logCatch('reconnect listAgents', e, s);
      refreshError = e;
    }
    try {
      projects = await catalog.listProjects();
      final id = selectedProjectId;
      // A concrete selection that vanished from the catalog falls back to the
      // default project. A null selection is the empty workspace and stays.
      if (id != null && !projects.any((p) => p.id == id)) {
        selectedProjectId = _pickDefaultProjectId();
      }
    } on Object catch (e, s) {
      _logCatch('reconnect listProjects', e, s);
      refreshError ??= e;
    }
    try {
      // Listing without a project filter would return every project's
      // threads, so the empty workspace publishes none instead.
      final id = selectedProjectId;
      _publishThreads(
        id == null
            ? const <ThreadSummary>[]
            : await catalog.listThreads(projectId: id),
      );
    } on Object catch (e, s) {
      _logCatch('reconnect listThreads', e, s);
      refreshError ??= e;
    }
    try {
      providers = await catalog.listProviders();
    } on Object catch (e, s) {
      _logCatch('reconnect listProviders', e, s);
      refreshError ??= e;
    }
    await _refreshExporters();
    statusMessage = refreshError == null ? null : formatChatError(refreshError);
    notifyListeners();
  }

  /// Creates a thread in [projectId], or the selected project when omitted.
  ///
  /// Creating under another project activates that project and selects the
  /// new thread so the workspace switches with the sidebar action.
  Future<void> createThread({String? projectId}) async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    final targetId = projectId ?? selectedProjectId;
    try {
      final created = await catalog.createThread(projectId: targetId);
      if (targetId != null &&
          targetId.isNotEmpty &&
          targetId != selectedProjectId) {
        await selectProject(targetId, preferThreadId: created.id);
        return;
      }
      threads.insert(0, created);
      _cacheSelectedThreads();
      notifyListeners();
      await selectThread(created.id);
    } on Object catch (e, s) {
      _logCatch('createThread', e, s);
      statusMessage = formatChatError(e);
      notifyListeners();
    }
  }

  String? _pickDefaultProjectId() {
    if (projects.isEmpty) {
      return null;
    }
    for (final p in projects) {
      if (p.name == 'Default') {
        return p.id;
      }
    }
    return projects.first.id;
  }

  /// Startup project: the remembered active project when it still exists in
  /// the catalog, else the default pick.
  Future<String?> _restoreInitialProjectId() async {
    final remembered = await preferredProject?.call();
    if (remembered != null &&
        remembered.isNotEmpty &&
        projects.any((p) => p.id == remembered)) {
      return remembered;
    }
    return _pickDefaultProjectId();
  }

  Future<void> selectProject(String id, {String? preferThreadId}) async {
    final catalog = _catalog;
    if (catalog == null || id == selectedProjectId) {
      return;
    }
    _parkLiveTranscriptIfNeeded();
    selectedProjectId = id;
    selectedThreadId = null;
    messages.clear();
    selectedAgentId = null;
    _sessionReady = false;
    // Notify first so the shell can swap parked workspaces immediately.
    notifyListeners();
    try {
      _publishThreads(await catalog.listThreads(projectId: id));
    } on Object catch (e, s) {
      _logCatch('selectProject listThreads', e, s);
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    await _refreshExporters();
    notifyListeners();
    final opening = await _preferredThreadId(id, explicit: preferThreadId);
    if (opening != null) {
      await selectThread(opening);
    }
  }

  /// Clears the active project without picking another one.
  ///
  /// Closing the last project tab calls this so the workspace shows its
  /// empty state; opening any thread from the sidebar activates its project
  /// again.
  Future<void> clearProjectSelection() async {
    _parkLiveTranscriptIfNeeded();
    selectedProjectId = null;
    selectedThreadId = null;
    messages.clear();
    selectedAgentId = null;
    _sessionReady = false;
    _publishThreads(const []);
    await _refreshExporters();
    notifyListeners();
  }

  /// Loads threads for a sidebar group that is not the active project.
  Future<void> ensureProjectThreads(String projectId) async {
    if (projectId.isEmpty || projectId == selectedProjectId) {
      return;
    }
    if (projectThreads.containsKey(projectId)) {
      return;
    }
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      projectThreads[projectId] = await catalog.listThreads(
        projectId: projectId,
      );
    } on Object catch (e, s) {
      _logCatch('ensureProjectThreads', e, s);
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  List<ThreadSummary> threadsFor(String projectId) {
    if (projectId == selectedProjectId) {
      return threads;
    }
    return projectThreads[projectId] ?? const [];
  }

  Future<String?> _preferredThreadId(
    String? projectId, {
    String? explicit,
  }) async {
    if (projectId == null || projectId.isEmpty || threads.isEmpty) {
      return null;
    }
    final requested = explicit ?? await preferredThread?.call(projectId);
    if (requested != null && threads.any((t) => t.id == requested)) {
      return requested;
    }
    return threads.first.id;
  }

  void _publishThreads(List<ThreadSummary> next) {
    threads = next;
    _cacheSelectedThreads();
  }

  void _cacheSelectedThreads() {
    final id = selectedProjectId;
    if (id == null || id.isEmpty) {
      return;
    }
    projectThreads[id] = List<ThreadSummary>.of(threads);
  }

  Future<void> createProject(String name) async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      final created = await catalog.createProject(name: name);
      projects = [...projects, created];
      await selectProject(created.id);
    } on Object catch (e, s) {
      _logCatch('createProject', e, s);
      statusMessage = formatChatError(e);
      notifyListeners();
    }
  }

  Future<void> _refreshExporters() async {
    final catalog = _catalog;
    final id = selectedProjectId;
    if (catalog == null || id == null || id.isEmpty) {
      exporters = List.of(ExportMethod.defaults);
      return;
    }
    try {
      final listed = await catalog.listExporters(id);
      exporters = listed.isEmpty ? List.of(ExportMethod.defaults) : listed;
    } on Object catch (e, s) {
      _logCatch('listExporters', e, s);
      exporters = List.of(ExportMethod.defaults);
    }
  }

  /// Exports or publishes the selected project.
  ///
  /// Archive methods trigger a browser download and return null. Publish
  /// methods return [ExportPublishResult] for the caller to present links.
  Future<ExportPublishResult?> exportSelectedProject({
    String method = 'download',
  }) async {
    final catalog = _catalog;
    final id = selectedProjectId;
    if (catalog == null || id == null || id.isEmpty) {
      return null;
    }
    final chosen = exporters.where((m) => m.id == method);
    if (chosen.isNotEmpty && !chosen.first.enabled) {
      return null;
    }
    try {
      final outcome = await catalog.exportProject(id, method: method);
      switch (outcome) {
        case ExportArchiveOutcome(:final archive):
          await _saveExport(archive.filename, archive.bytes);
          return null;
        case ExportPublishOutcome(:final result):
          return result;
      }
    } on Object catch (e, s) {
      _logCatch('exportSelectedProject', e, s);
      statusMessage = formatChatError(e);
      notifyListeners();
      rethrow;
    }
  }

  Future<void> reloadAgents() async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      agents = await catalog.listAgents();
    } on Object catch (e, s) {
      _logCatch('reloadAgents listAgents', e, s);
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    try {
      projects = await catalog.listProjects();
    } on Object catch (e, s) {
      _logCatch('reloadAgents listProjects', e, s);
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    // A concrete selection that vanished from the catalog falls back to the
    // default project. A null selection is the empty workspace and stays.
    final selectedId = selectedProjectId;
    final selectedMissing =
        selectedId != null && !projects.any((p) => p.id == selectedId);
    if (selectedMissing) {
      final next = _pickDefaultProjectId();
      if (next == null) {
        selectedProjectId = null;
        selectedThreadId = null;
        messages.clear();
        _publishThreads(const []);
        selectedAgentId = null;
        _sessionReady = false;
        notifyListeners();
        return;
      }
      if (next != selectedProjectId) {
        await selectProject(next);
      }
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
    } on Object catch (e, s) {
      _logCatch('renameThread', e, s);
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  Future<void> setThreadViewMode(String? modeId) async {
    final id = selectedThreadId;
    final catalog = _catalog;
    if (id == null || catalog == null) {
      return;
    }
    final previous = selectedThread?.viewModeId;
    _mapThread(id, (t) => t.copyWith(viewModeId: modeId));
    notifyListeners();
    try {
      final updated = await catalog.patchThreadViewMode(id, modeId);
      _replaceThread(updated);
      notifyListeners();
    } on Object catch (e, s) {
      _logCatch('setThreadViewMode', e, s);
      _mapThread(id, (t) => t.copyWith(viewModeId: previous));
      statusMessage = formatChatError(e);
      notifyListeners();
      rethrow;
    }
  }

  Future<void> selectThread(String id) async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    // Do not cancel an in-flight turn on navigation — pending permission /
    // ask_user stays alive so returning to the thread can finish it.
    _threadLoadEpoch++;
    final loadGen = _threadLoadEpoch;
    _sessionStarting = false;
    final ThreadDetail detail;
    try {
      detail = await catalog.getThread(id);
    } on CatalogException catch (e, s) {
      _logCatch('selectThread getThread', e, s);
      if (loadGen != _threadLoadEpoch) {
        return;
      }
      if (e.statusCode == 404) {
        List<ThreadSummary> refreshed;
        try {
          refreshed = await catalog.listThreads(projectId: selectedProjectId);
        } on Object catch (listErr, listStack) {
          _logCatch('selectThread listThreads after 404', listErr, listStack);
          if (loadGen != _threadLoadEpoch) {
            return;
          }
          statusMessage = formatChatError(listErr);
          notifyListeners();
          return;
        }
        if (loadGen != _threadLoadEpoch) {
          return;
        }
        _parkLiveTranscriptIfNeeded();
        selectedThreadId = null;
        messages.clear();
        selectedAgentId = null;
        _sessionReady = false;
        _publishThreads(refreshed);
        statusMessage = formatChatError(e);
        notifyListeners();
        return;
      }
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    } on Object catch (e, s) {
      _logCatch('selectThread getThread', e, s);
      if (loadGen != _threadLoadEpoch) {
        return;
      }
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    if (loadGen != _threadLoadEpoch) {
      return;
    }
    if (selectedThreadId != null && selectedThreadId != id) {
      _parkLiveTranscriptIfNeeded();
    }
    selectedThreadId = id;
    _replaceThread(detail.thread);
    final keepLiveTranscript =
        _sessionOwnerThreadId == id &&
        (_sending || _pendingByThread.containsKey(id));
    if (keepLiveTranscript) {
      if (_parkedLiveThreadId == id) {
        _unparkLiveTranscriptIntoMessages();
      }
    } else {
      messages
        ..clear()
        ..addAll(detail.messages.expand(bubblesFromThreadMessage));
    }
    final agentId = detail.thread.agentId;
    if (agentId != null) {
      selectedAgentId = agentId;
      if (!selectedAgentIsComplete) {
        _sessionReady = false;
        notifyListeners();
        return;
      }
      final ownerBusy =
          _sessionOwnerThreadId != null &&
          _sessionOwnerThreadId != id &&
          (_sending || _pendingByThread.isNotEmpty);
      if (ownerBusy) {
        _sessionReady = false;
        notifyListeners();
        return;
      }
      if (_sessionOwnerThreadId == id && _sessionReady) {
        notifyListeners();
        return;
      }
      if (_sessionOwnerThreadId == id &&
          (_sending || _pendingByThread.containsKey(id))) {
        _sessionReady = true;
        notifyListeners();
        return;
      }
      _sessionStarting = true;
      _sessionReady = false;
      notifyListeners();
      try {
        await _session.startSession(agentId, threadId: id);
        if (loadGen != _threadLoadEpoch || selectedThreadId != id) {
          return;
        }
        selectedAgentId = agentId;
        _sessionOwnerThreadId = id;
        _sessionReady = true;
        status = ChatStatus.connected;
        statusMessage = null;
      } on Object catch (e, s) {
        _logCatch('selectThread startSession', e, s);
        if (loadGen != _threadLoadEpoch || selectedThreadId != id) {
          return;
        }
        selectedAgentId = null;
        _sessionReady = false;
        statusMessage = formatChatError(e);
      } finally {
        if (loadGen == _threadLoadEpoch) {
          _sessionStarting = false;
          notifyListeners();
        }
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
      _sessionOwnerThreadId = threadId;
      _sessionReady = true;
      status = ChatStatus.connected;
      statusMessage = null;
    } on Object catch (e, s) {
      _logCatch('startSession', e, s);
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
    } on Object catch (e, s) {
      _logCatch('selectModel', e, s);
      statusMessage = formatChatError(e);
    }
    notifyListeners();
  }

  Future<void> send(String text) async {
    final trimmed = text.trim();
    if (!canSend || trimmed.isEmpty) return;

    final epoch = ++_sendEpoch;
    final live = _liveMessages;
    _uncommittedStart = live.length;
    live.add(
      ChatBubble(
        kind: ChatBubbleKind.user,
        text: trimmed,
        createdAt: DateTime.now(),
      ),
    );
    _sending = true;
    notifyListeners();

    try {
      await _session.sendPrompt(
        trimmed,
        onEvent: (event) {
          if (epoch != _sendEpoch) {
            return;
          }
          switch (event) {
            case AgentThoughtDelta(:final text):
              _growOrAppend(
                ChatBubbleKind.thought,
                append: text,
                streamingThought: true,
              );
            case AgentToolCallEvent():
              _upsertToolCall(event);
            case AgentMessageDelta(:final text):
              _growOrAppend(
                ChatBubbleKind.message,
                append: text,
                model: currentModel,
                providerName: _selectedProviderName(),
              );
            case AgentUsageEvent(:final usage):
              _growOrAppend(
                ChatBubbleKind.stats,
                usage: usage,
                stopReason: usage.stopReason,
              );
              _stampPredictedPerSecond(usage.predictedPerSecond);
          }
          notifyListeners();
        },
      );
      if (epoch != _sendEpoch) {
        return;
      }
      final after = _liveMessages;
      for (var i = 0; i < after.length; i++) {
        if (after[i].kind == ChatBubbleKind.thought &&
            after[i].streamingThought) {
          after[i] = after[i].copyWith(streamingThought: false);
        }
      }
      final wroteFiles = _turnWroteFiles();
      _sending = false;
      notifyListeners();
      try {
        await _refreshSelectedThread(
          optimisticTitle: _autoTitle(trimmed),
          epoch: epoch,
        );
      } on Object catch (e, s) {
        _logCatch('send refreshSelectedThread', e, s);
        statusMessage = formatChatError(e);
      }
      if (wroteFiles) {
        onAgentTurnCommitted?.call();
      }
    } on Object catch (e, s) {
      _logCatch('send', e, s);
      if (epoch != _sendEpoch) {
        return;
      }
      _dropUncommitted();
      status = ChatStatus.error;
      statusMessage = formatChatError(e);
    } finally {
      if (epoch == _sendEpoch) {
        _sending = false;
        if (!_ownerTurnLive) {
          _clearParkedLiveTranscript();
        }
      }
      notifyListeners();
    }
  }

  Future<void> _refreshSelectedThread({
    required String optimisticTitle,
    int? epoch,
  }) async {
    final catalog = _catalog;
    final id = _sessionOwnerThreadId ?? selectedThreadId;
    if (catalog == null || id == null) {
      return;
    }
    final loadGen = _threadLoadEpoch;
    final detail = await catalog.getThread(id);
    if (loadGen != _threadLoadEpoch || (epoch != null && epoch != _sendEpoch)) {
      return;
    }
    // Thread list may still reference this id even if the user navigated away.
    ThreadSummary? local;
    for (final t in threads) {
      if (t.id == id) {
        local = t;
        break;
      }
    }
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
    final viewingOwner = selectedThreadId == id;
    if (viewingOwner || threads.any((t) => t.id == id)) {
      _replaceThread(summary, promote: viewingOwner);
    }
    if (detail.messages.isNotEmpty) {
      final bubbles = detail.messages.expand(bubblesFromThreadMessage).toList();
      if (viewingOwner) {
        messages
          ..clear()
          ..addAll(bubbles);
        _clearParkedLiveTranscript();
      } else if (_parkedLiveThreadId == id) {
        _parkedLiveMessages = bubbles;
      }
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
    _cacheSelectedThreads();
  }

  void _mapThread(String id, ThreadSummary Function(ThreadSummary) map) {
    threads = [
      for (final t in threads)
        if (t.id == id) map(t) else t,
    ];
    _cacheSelectedThreads();
  }

  ThreadSummary _copyThread(
    ThreadSummary t, {
    String? title,
    String? titleSource,
    String? agentId,
    DateTime? updatedAt,
    String? viewModeId,
  }) {
    return ThreadSummary(
      id: t.id,
      title: title ?? t.title,
      titleSource: titleSource ?? t.titleSource,
      agentId: agentId ?? t.agentId,
      currentModel: t.currentModel,
      messageCount: t.messageCount,
      viewModeId: viewModeId ?? t.viewModeId,
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

  void _growOrAppend(
    ChatBubbleKind kind, {
    String append = '',
    String? model,
    String? providerName,
    TurnUsage? usage,
    String? stopReason,
    bool? streamingThought,
  }) {
    final live = _liveMessages;
    if (live.isNotEmpty && live.last.kind == kind) {
      final last = live.last;
      live[live.length - 1] = last.copyWith(
        text: last.text + append,
        model: model ?? last.model,
        providerName: providerName ?? last.providerName,
        usage: usage ?? last.usage,
        stopReason: stopReason ?? last.stopReason,
        streamingThought: streamingThought ?? last.streamingThought,
      );
      return;
    }
    live.add(
      ChatBubble(
        kind: kind,
        text: append,
        model: model,
        providerName: providerName,
        usage: usage,
        stopReason: stopReason,
        streamingThought: streamingThought ?? false,
        createdAt: kind == ChatBubbleKind.message ? DateTime.now() : null,
      ),
    );
  }

  void _upsertToolCall(AgentToolCallEvent event) {
    final live = _liveMessages;
    final index = live.indexWhere(
      (bubble) =>
          bubble.kind == ChatBubbleKind.toolCall &&
          bubble.toolCallId == event.id,
    );
    if (index < 0) {
      live.add(
        ChatBubble(
          kind: ChatBubbleKind.toolCall,
          toolCallId: event.id,
          toolTitle: event.title ?? 'Tool call',
          toolStatus: event.status,
          toolInput: event.rawInput,
          toolOutput: event.rawOutput,
          streamingTool: event.inProgress,
        ),
      );
      return;
    }
    final previous = live[index];
    final status = event.status ?? previous.toolStatus;
    live[index] = previous.copyWith(
      toolTitle: event.title,
      toolStatus: event.status,
      toolInput: event.rawInput,
      toolOutput: event.rawOutput,
      streamingTool: status == null
          ? event.inProgress
          : status != 'completed' && status != 'failed',
    );
  }

  bool _turnWroteFiles() {
    final live = _liveMessages;
    for (var i = _uncommittedStart; i < live.length; i++) {
      final bubble = live[i];
      if (bubble.kind != ChatBubbleKind.toolCall) {
        continue;
      }
      final title = (bubble.toolTitle ?? '').toLowerCase();
      if (title.contains('write file') || title.contains('write_file')) {
        return true;
      }
    }
    return false;
  }

  void _stampPredictedPerSecond(double? tok) {
    if (tok == null) {
      return;
    }
    final live = _liveMessages;
    for (var i = live.length - 1; i >= 0; i--) {
      if (live[i].kind == ChatBubbleKind.message) {
        live[i] = live[i].copyWith(predictedPerSecond: tok);
        return;
      }
    }
  }

  void _dropUncommitted() {
    final live = _liveMessages;
    if (live.length > _uncommittedStart) {
      live.removeRange(_uncommittedStart, live.length);
    }
  }

  @override
  void dispose() {
    final sub = _stateSub;
    _stateSub = null;
    if (sub != null) {
      unawaited(
        sub.cancel().catchError((Object e, StackTrace s) {
          AppLog.record('stateSub cancel: $e', s);
        }),
      );
    }
    unawaited(
      _session.close().catchError((Object e, StackTrace s) {
        AppLog.record('session close: $e', s);
      }),
    );
    super.dispose();
  }
}
