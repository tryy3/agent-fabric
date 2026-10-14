/// One built-in gate question as listed by `GET /v1/permissions/questions`.
class GateQuestion {
  const GateQuestion({
    required this.id,
    required this.family,
    required this.kind,
    required this.text,
    required this.enabled,
    this.tools = const [],
    this.risk,
  });

  final String id;
  final String family;
  final List<String> tools;

  /// `tripwire` or `policy`.
  final String kind;
  final String text;

  /// Null for a question that carries no score (`policy_exception`).
  final int? risk;
  final bool enabled;

  bool get isPolicy => kind == 'policy';

  factory GateQuestion.fromJson(Map<String, Object?> json) {
    final tools = json['tools'];
    return GateQuestion(
      id: json['id'] as String? ?? '',
      family: json['family'] as String? ?? '',
      kind: json['kind'] as String? ?? 'tripwire',
      text: json['text'] as String? ?? '',
      enabled: json['enabled'] != false,
      risk: (json['risk'] as num?)?.toInt(),
      tools: tools is List ? [for (final t in tools) t.toString()] : const [],
    );
  }
}

/// A tool family: the tools one question set covers.
class GateQuestionFamily {
  const GateQuestionFamily({required this.id, this.tools = const []});

  final String id;
  final List<String> tools;
}

/// The editing limits the plane enforces on gate questions.
class GateQuestionLimits {
  const GateQuestionLimits({
    this.idPattern = r'^[a-z][a-z0-9_]{1,31}$',
    this.minTextChars = 10,
    this.maxTextChars = 600,
    this.maxCustomPerFamily = 8,
    this.minRisk = 1,
    this.maxRisk = 10,
  });

  final String idPattern;
  final int minTextChars;
  final int maxTextChars;
  final int maxCustomPerFamily;
  final int minRisk;
  final int maxRisk;

  factory GateQuestionLimits.fromJson(Object? raw) {
    if (raw is! Map) return const GateQuestionLimits();
    const d = GateQuestionLimits();
    return GateQuestionLimits(
      idPattern: raw['idPattern'] as String? ?? d.idPattern,
      minTextChars: (raw['minTextChars'] as num?)?.toInt() ?? d.minTextChars,
      maxTextChars: (raw['maxTextChars'] as num?)?.toInt() ?? d.maxTextChars,
      maxCustomPerFamily:
          (raw['maxCustomPerFamily'] as num?)?.toInt() ?? d.maxCustomPerFamily,
      minRisk: (raw['minRisk'] as num?)?.toInt() ?? d.minRisk,
      maxRisk: (raw['maxRisk'] as num?)?.toInt() ?? d.maxRisk,
    );
  }
}

/// The body of `GET /v1/permissions/questions`.
class GateQuestionCatalog {
  const GateQuestionCatalog({
    this.questions = const [],
    this.families = const [],
    this.limits = const GateQuestionLimits(),
  });

  final List<GateQuestion> questions;
  final List<GateQuestionFamily> families;
  final GateQuestionLimits limits;

  factory GateQuestionCatalog.fromJson(Map<String, Object?> json) {
    final questions = json['questions'];
    final families = json['families'];
    return GateQuestionCatalog(
      questions: [
        if (questions is List)
          for (final q in questions)
            if (q is Map) GateQuestion.fromJson(Map<String, Object?>.from(q)),
      ],
      families: [
        if (families is List)
          for (final f in families)
            if (f is Map)
              GateQuestionFamily(
                id: f['id'] as String? ?? '',
                tools: [
                  if (f['tools'] case final List tools)
                    for (final t in tools) t.toString(),
                ],
              ),
      ],
      limits: GateQuestionLimits.fromJson(json['limits']),
    );
  }

  /// The ids of the families a custom question for [tools] belongs to.
  Set<String> familiesOf(Iterable<String> tools) => {
    for (final f in families)
      if (f.tools.any(tools.contains) || tools.contains(f.id)) f.id,
  };
}

/// The user's change to one built-in question; a null field is the default.
class GateQuestionOverride {
  GateQuestionOverride({this.enabled, this.risk, this.text});

  bool? enabled;
  int? risk;
  String? text;

  bool get isEmpty => enabled == null && risk == null && text == null;

  Map<String, Object?> toJson() => {
    if (enabled != null) 'enabled': enabled,
    if (risk != null) 'risk': risk,
    if (text != null) 'text': text,
  };
}

/// A tripwire the user wrote.
class CustomGateQuestion {
  CustomGateQuestion({
    required this.id,
    required this.tools,
    required this.text,
    required this.risk,
    this.enabled = true,
  });

  String id;
  List<String> tools;
  String text;
  int risk;
  bool enabled;

  factory CustomGateQuestion.fromJson(Map<String, Object?> json) {
    final tools = json['tools'];
    return CustomGateQuestion(
      id: json['id'] as String? ?? '',
      tools: tools is List ? [for (final t in tools) t.toString()] : [],
      text: json['text'] as String? ?? '',
      risk: (json['risk'] as num?)?.toInt() ?? 5,
      enabled: json['enabled'] != false,
    );
  }

  CustomGateQuestion copy() => CustomGateQuestion(
    id: id,
    tools: List.of(tools),
    text: text,
    risk: risk,
    enabled: enabled,
  );

  Map<String, Object?> toJson() => {
    'id': id,
    'tools': tools,
    'kind': 'tripwire',
    'text': text,
    'risk': risk,
    'enabled': enabled,
  };
}

/// The editable `permissions.questions` of the plane or one assistant. Only
/// the difference from the defaults is stored.
class GateQuestionsDraft {
  GateQuestionsDraft({
    Map<String, GateQuestionOverride>? builtins,
    List<CustomGateQuestion>? custom,
  }) : builtins = builtins ?? {},
       custom = custom ?? [];

  final Map<String, GateQuestionOverride> builtins;
  final List<CustomGateQuestion> custom;

  factory GateQuestionsDraft.fromJson(Object? raw) {
    if (raw is! Map) return GateQuestionsDraft();
    final builtins = raw['builtins'];
    final custom = raw['custom'];
    return GateQuestionsDraft(
      builtins: {
        if (builtins is Map)
          for (final e in builtins.entries)
            if (e.value is Map)
              e.key.toString(): GateQuestionOverride(
                enabled: (e.value as Map)['enabled'] as bool?,
                risk: ((e.value as Map)['risk'] as num?)?.toInt(),
                text: (e.value as Map)['text'] as String?,
              ),
      },
      custom: [
        if (custom is List)
          for (final c in custom)
            if (c is Map)
              CustomGateQuestion.fromJson(Map<String, Object?>.from(c)),
      ],
    );
  }

  bool isModified(String id) => builtins.containsKey(id);

  String textOf(GateQuestion q) => builtins[q.id]?.text ?? q.text;
  int? riskOf(GateQuestion q) => builtins[q.id]?.risk ?? q.risk;
  bool enabledOf(GateQuestion q) => builtins[q.id]?.enabled ?? q.enabled;

  GateQuestionOverride _edit(String id) =>
      builtins.putIfAbsent(id, GateQuestionOverride.new);

  void _prune(String id) {
    if (builtins[id]?.isEmpty ?? false) builtins.remove(id);
  }

  void setText(GateQuestion q, String value) {
    _edit(q.id).text = value == q.text ? null : value;
    _prune(q.id);
  }

  void setRisk(GateQuestion q, int value) {
    _edit(q.id).risk = value == q.risk ? null : value;
    _prune(q.id);
  }

  void setEnabled(GateQuestion q, {required bool value}) {
    _edit(q.id).enabled = value == q.enabled ? null : value;
    _prune(q.id);
  }

  void reset(String id) => builtins.remove(id);

  /// The `permissions.questions` value, or null when nothing is changed.
  Map<String, Object?>? toJson() {
    if (builtins.isEmpty && custom.isEmpty) return null;
    return {
      if (builtins.isNotEmpty)
        'builtins': {for (final e in builtins.entries) e.key: e.value.toJson()},
      if (custom.isNotEmpty) 'custom': [for (final c in custom) c.toJson()],
    };
  }
}
