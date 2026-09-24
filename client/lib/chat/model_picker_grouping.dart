import '../acp/agent_connection.dart';
import '../catalog/models.dart';

const kOtherProviderGroupName = 'Other';

class ModelProviderGroup {
  const ModelProviderGroup({
    this.providerId,
    required this.providerName,
    required this.models,
  });

  final String? providerId;
  final String providerName;
  final List<ModelOption> models;
}

List<ModelProviderGroup> groupModelsByProvider({
  required List<ModelOption> models,
  required List<Provider> providers,
}) {
  final idToProvider = <String, Provider>{};
  for (final p in providers) {
    for (final m in p.models) {
      idToProvider.putIfAbsent(m.id, () => p);
    }
  }

  final byProvider = <String, ModelProviderGroup>{};
  final other = <ModelOption>[];

  for (final model in models) {
    final p = idToProvider[model.id];
    if (p == null) {
      other.add(model);
      continue;
    }
    final existing = byProvider[p.id];
    if (existing == null) {
      byProvider[p.id] = ModelProviderGroup(
        providerId: p.id,
        providerName: p.name,
        models: [model],
      );
    } else {
      byProvider[p.id] = ModelProviderGroup(
        providerId: existing.providerId,
        providerName: existing.providerName,
        models: [...existing.models, model],
      );
    }
  }

  final groups = byProvider.values.toList();
  // Preserve provider list order for matched groups.
  groups.sort((a, b) {
    final ai = providers.indexWhere((p) => p.id == a.providerId);
    final bi = providers.indexWhere((p) => p.id == b.providerId);
    return ai.compareTo(bi);
  });
  if (other.isNotEmpty) {
    groups.add(
      ModelProviderGroup(providerName: kOtherProviderGroupName, models: other),
    );
  }
  return groups;
}

List<ModelProviderGroup> filterModelGroups({
  required List<ModelProviderGroup> groups,
  required String query,
}) {
  final q = query.trim().toLowerCase();
  if (q.isEmpty) return groups;
  final out = <ModelProviderGroup>[];
  for (final g in groups) {
    final models = [
      for (final m in g.models)
        if (m.name.toLowerCase().contains(q) || m.id.toLowerCase().contains(q))
          m,
    ];
    if (models.isNotEmpty) {
      out.add(
        ModelProviderGroup(
          providerId: g.providerId,
          providerName: g.providerName,
          models: models,
        ),
      );
    }
  }
  return out;
}
