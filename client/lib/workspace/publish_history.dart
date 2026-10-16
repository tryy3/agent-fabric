import '../catalog/models.dart';

/// Publishes that share one publish provider and provider-side target (for
/// Netlify, one site), newest first.
class PublishGroup {
  const PublishGroup({
    required this.provider,
    required this.providerLabel,
    required this.target,
    required this.publishes,
    required this.showTarget,
  });

  final String provider;
  final String providerLabel;
  final String target;

  /// Newest first; never empty.
  final List<PublishRecord> publishes;

  /// Whether the provider has more than one target in the history, so the
  /// target must be shown to tell the groups apart.
  final bool showTarget;

  /// The provider's always-latest link, taken from the live deploy, else the
  /// newest publish that reported one.
  String get latestUrl {
    for (final publish in publishes) {
      if (publish.live && publish.siteUrl.isNotEmpty) {
        return publish.siteUrl;
      }
    }
    for (final publish in publishes) {
      if (publish.siteUrl.isNotEmpty) {
        return publish.siteUrl;
      }
    }
    return '';
  }

  DateTime get lastPublishedAt => publishes.first.createdAt;
}

/// Groups [publishes] by provider and target. Groups are ordered by their
/// most recent publish; each group keeps its publishes newest first.
List<PublishGroup> groupPublishes(List<PublishRecord> publishes) {
  final sorted = [...publishes]
    ..sort((a, b) => b.createdAt.compareTo(a.createdAt));
  final byKey = <(String, String), List<PublishRecord>>{};
  final targetsPerProvider = <String, Set<String>>{};
  for (final publish in sorted) {
    byKey
        .putIfAbsent((publish.provider, publish.target), () => [])
        .add(publish);
    targetsPerProvider
        .putIfAbsent(publish.provider, () => <String>{})
        .add(publish.target);
  }
  return [
    for (final entry in byKey.entries)
      PublishGroup(
        provider: entry.key.$1,
        providerLabel: entry.value.first.providerLabel,
        target: entry.key.$2,
        publishes: List.unmodifiable(entry.value),
        showTarget: (targetsPerProvider[entry.key.$1]?.length ?? 0) > 1,
      ),
  ];
}

/// Distinct providers in [publishes] with their publish counts, ordered by
/// most recent publish.
List<({String provider, String label, int count})> publishProviders(
  List<PublishRecord> publishes,
) {
  final sorted = [...publishes]
    ..sort((a, b) => b.createdAt.compareTo(a.createdAt));
  final labels = <String, String>{};
  final counts = <String, int>{};
  for (final publish in sorted) {
    labels.putIfAbsent(publish.provider, () => publish.providerLabel);
    counts[publish.provider] = (counts[publish.provider] ?? 0) + 1;
  }
  return [
    for (final entry in labels.entries)
      (provider: entry.key, label: entry.value, count: counts[entry.key] ?? 0),
  ];
}
