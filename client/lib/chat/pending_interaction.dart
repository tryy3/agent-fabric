import 'dart:async';

import 'package:acpd/acpd.dart';

import 'ask_user_question.dart';

/// A mid-turn human interaction waiting on the user for one thread.
sealed class PendingInteraction {
  const PendingInteraction({required this.threadId});

  final String threadId;
}

/// ACP [session/request_permission] waiting above the composer.
final class PendingPermission extends PendingInteraction {
  PendingPermission({
    required super.threadId,
    required this.request,
    required this.completer,
  });

  final RequestPermissionRequest request;
  final Completer<RequestPermissionResponse> completer;

  String get title => request.toolCall.title ?? 'Permission required';

  String get reason {
    final raw = request.toolCall.rawInput;
    if (raw is Map) {
      final reason = raw['reason'];
      if (reason is String && reason.trim().isNotEmpty) {
        return reason.trim();
      }
      final path = raw['path'];
      if (path is String && path.trim().isNotEmpty) {
        return 'Path: ${path.trim()}';
      }
    }
    return '';
  }
}

/// ACP elicitation / ask_user waiting above the composer.
final class PendingAskUser extends PendingInteraction {
  PendingAskUser({
    required super.threadId,
    required this.message,
    required this.questions,
    required this.completer,
  });

  final String message;
  final List<AskUserQuestion> questions;
  final Completer<Map<String, Object?>> completer;
}
