import 'dart:async';

import 'package:acpd/acpd.dart';

import '../acp/gate_info.dart';
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

  /// Command line of a run_command request, or null for other tools.
  String? get commandLine {
    final raw = request.toolCall.rawInput;
    if (raw is! Map) return null;
    final command = raw['command'];
    if (command is! List || command.isEmpty) return null;
    return command.map((part) => part.toString()).join(' ');
  }

  /// What the gate concluded, e.g. `risk 6 · elevated · rules 6 → llm 4`, or
  /// null when the request carries no score.
  String? get riskLine {
    final raw = request.toolCall.rawInput;
    if (raw is! Map) return null;
    final risk = raw['risk'];
    if (risk is! num) return null;
    final band = raw['band'];
    final scores = GateScore.listFrom(raw['scores']);
    return [
      'risk ${risk.toInt()}',
      if (band is String && band.isNotEmpty) band,
      if (scores.length > 1) scores.map((s) => s.trailLabel).join(' → '),
    ].join(' · ');
  }

  /// Why the gate asks: the scorer's rationale, else the rule's reason.
  String? get why {
    final raw = request.toolCall.rawInput;
    if (raw is! Map) return null;
    for (final key in const ['rationale', 'reason']) {
      final value = raw[key];
      if (value is String && value.trim().isNotEmpty) return value.trim();
    }
    return null;
  }

  String get reason {
    final raw = request.toolCall.rawInput;
    final commandLine = this.commandLine;
    if (raw is Map && commandLine != null) {
      final cwd = raw['cwd'];
      final where = cwd is String && cwd.trim().isNotEmpty && cwd != '.'
          ? '\nin ${cwd.trim()}'
          : '';
      return '\$ $commandLine$where';
    }
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
