/// One built-in rule tier of the tool gate, as listed by
/// `GET /v1/permissions/builtins`.
class PermissionTier {
  const PermissionTier({
    required this.id,
    required this.title,
    required this.description,
    required this.action,
    required this.risk,
    required this.consult,
    this.programs,
    this.locked = false,
  });

  final String id;
  final String title;
  final String description;

  /// `allow`, `ask` or `deny`.
  final String action;
  final int risk;

  /// Whether configured scorers are asked about this tier's calls.
  final bool consult;

  /// Program names of a program-list tier; null for other tiers.
  final List<String>? programs;

  /// Locked tiers are structural protections and cannot be changed.
  final bool locked;

  factory PermissionTier.fromJson(Map<String, Object?> json) {
    final programs = json['programs'];
    return PermissionTier(
      id: json['id'] as String? ?? '',
      title: json['title'] as String? ?? '',
      description: json['description'] as String? ?? '',
      action: json['action'] as String? ?? 'ask',
      risk: (json['risk'] as num?)?.toInt() ?? 0,
      consult: json['consult'] == true,
      programs: programs is List
          ? [for (final p in programs) p.toString()]
          : null,
      locked: json['locked'] == true,
    );
  }
}

/// A tier with the user's override applied, as edited in settings.
class PermissionTierDraft {
  PermissionTierDraft(this.tier, [Map<String, Object?>? override])
    : risk = (override?['risk'] as num?)?.toInt(),
      consult = override?['consult'] as bool?,
      added = {
        if (override?['add'] case final List list)
          for (final p in list) p.toString(),
      },
      removed = {
        if (override?['remove'] case final List list)
          for (final p in list) p.toString(),
      };

  final PermissionTier tier;

  /// Overridden base score, or null for the default.
  int? risk;

  /// Overridden scorer consultation, or null for the default.
  bool? consult;
  final Set<String> added;
  final Set<String> removed;

  int get effectiveRisk => risk ?? tier.risk;
  bool get effectiveConsult => consult ?? tier.consult;

  /// The tier's programs with additions and removals applied, sorted.
  List<String> get programs {
    final defaults = tier.programs ?? const <String>[];
    return {...defaults.where((p) => !removed.contains(p)), ...added}.toList()
      ..sort();
  }

  bool get isModified =>
      risk != null || consult != null || added.isNotEmpty || removed.isNotEmpty;

  void setRisk(int value) => risk = value == tier.risk ? null : value;

  void setConsult({required bool value}) =>
      consult = value == tier.consult ? null : value;

  /// Adds a program; returns false when the name is not a bare program name.
  bool addProgram(String name) {
    final program = name.trim();
    if (program.isEmpty || RegExp(r'[\s/]').hasMatch(program)) return false;
    if (removed.remove(program)) return true;
    if (!(tier.programs ?? const []).contains(program)) added.add(program);
    return true;
  }

  void removeProgram(String program) {
    if (added.remove(program)) return;
    if ((tier.programs ?? const []).contains(program)) removed.add(program);
  }

  void reset() {
    risk = null;
    consult = null;
    added.clear();
    removed.clear();
  }

  /// The `permissions.builtins` entry, or null when nothing is overridden.
  Map<String, Object?>? toJson() {
    if (!isModified) return null;
    return {
      if (risk != null) 'risk': risk,
      if (consult != null) 'consult': consult,
      if (added.isNotEmpty) 'add': added.toList()..sort(),
      if (removed.isNotEmpty) 'remove': removed.toList()..sort(),
    };
  }
}
