/// What the control plane's tool gate decided for one tool call: the risk
/// score, the permission mode in force and what happened. Shown to the user
/// and stored with the transcript; the model never receives it.
class GateInfo {
  const GateInfo({
    required this.mode,
    required this.outcome,
    this.risk,
    this.band,
    this.verdict,
    this.ruleId,
    this.reason,
    this.rationale,
    this.source,
    this.scores = const [],
    this.userRule = false,
    this.tainted = false,
  });

  /// Permission mode: `ask`, `auto_approve`, `auto` or `full`.
  final String mode;

  /// `allowed`, `approved`, `approved_session`, `rejected`, `dismissed`,
  /// `denied` or `cancelled`.
  final String outcome;

  /// 1-10, null when the gate did not score the call.
  final int? risk;

  /// `safe`, `low`, `elevated`, `high` or `cancel`.
  final String? band;
  final String? verdict;
  final String? ruleId;
  final String? reason;
  final String? rationale;
  final String? source;
  final List<GateScore> scores;

  /// The verdict came from one of the user's permission rules.
  final bool userRule;

  /// The session had read web content when the call was gated.
  final bool tainted;

  static GateInfo? tryParse(Object? raw) {
    if (raw is! Map) return null;
    final json = Map<String, Object?>.from(raw);
    final scores = <GateScore>[];
    if (json['scores'] case final List list) {
      for (final item in list) {
        if (item is Map) {
          scores.add(GateScore.fromJson(Map<String, Object?>.from(item)));
        }
      }
    }
    return GateInfo(
      mode: json['mode'] as String? ?? '',
      outcome: json['outcome'] as String? ?? '',
      risk: (json['risk'] as num?)?.toInt(),
      band: json['band'] as String?,
      verdict: json['verdict'] as String?,
      ruleId: json['ruleId'] as String?,
      reason: json['reason'] as String?,
      rationale: json['rationale'] as String?,
      source: json['source'] as String?,
      scores: scores,
      userRule: json['userRule'] == true,
      tainted: json['tainted'] == true,
    );
  }

  /// Short label for the tool call header, e.g. `risk 6 · elevated`.
  String get badgeLabel {
    final r = risk;
    if (r == null) return outcomeLabel;
    return 'risk $r · ${band ?? ''}'.trim();
  }

  String get outcomeLabel => switch (outcome) {
    'allowed' => 'ran automatically',
    'approved' => 'approved',
    'approved_session' => 'approved for session',
    'rejected' => 'rejected',
    'dismissed' => 'dismissed',
    'denied' => 'blocked',
    'cancelled' => 'cancelled by gate',
    _ => outcome.replaceAll('_', ' '),
  };

  String get modeLabel => switch (mode) {
    'ask' => 'Ask for approval',
    'auto_approve' => 'Approve for me',
    'auto' => 'Run automatically',
    'full' => 'Full access',
    _ => mode,
  };
}

/// One evaluator's score within a [GateInfo].
class GateScore {
  const GateScore({
    required this.source,
    required this.risk,
    this.ruleId,
    this.rationale,
    this.confidence,
  });

  final String source;
  final int risk;
  final String? ruleId;
  final String? rationale;

  /// 0-1 when the evaluator reported how sure it was.
  final double? confidence;

  /// Short form for a score trail, e.g. `jev 3 (0.82)`.
  String get trailLabel {
    final c = confidence;
    final name = source.isEmpty ? 'evaluator' : source;
    return c == null ? '$name $risk' : '$name $risk (${c.toStringAsFixed(2)})';
  }

  factory GateScore.fromJson(Map<String, Object?> json) => GateScore(
    source: json['source'] as String? ?? '',
    risk: (json['risk'] as num?)?.toInt() ?? 0,
    ruleId: json['ruleId'] as String?,
    rationale: json['rationale'] as String?,
    confidence: (json['confidence'] as num?)?.toDouble(),
  );

  /// Parses a `scores` list as sent in gate metadata and permission requests.
  static List<GateScore> listFrom(Object? raw) => [
    if (raw is List)
      for (final item in raw)
        if (item is Map) GateScore.fromJson(Map<String, Object?>.from(item)),
  ];
}
