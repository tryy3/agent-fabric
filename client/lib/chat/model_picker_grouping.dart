import '../acp/agent_connection.dart';
import '../catalog/models.dart';

const kOtherInferenceConnectionGroupName = 'Other';

class ModelInferenceConnectionGroup {
  const ModelInferenceConnectionGroup({
    this.inferenceConnectionId,
    required this.inferenceConnectionName,
    required this.models,
  });

  final String? inferenceConnectionId;
  final String inferenceConnectionName;
  final List<ModelOption> models;
}

List<ModelInferenceConnectionGroup> groupModelsByInferenceConnection({
  required List<ModelOption> models,
  required List<InferenceConnection> inferenceConnections,
}) {
  final idToConnection = <String, InferenceConnection>{};
  final unsupportedIds = <String>{};
  for (final connection in inferenceConnections) {
    for (final m in connection.models) {
      if (!idToConnection.containsKey(m.id) && m.unsupported) {
        unsupportedIds.add(m.id);
      }
      idToConnection.putIfAbsent(m.id, () => connection);
    }
  }

  final byConnection = <String, ModelInferenceConnectionGroup>{};
  final other = <ModelOption>[];

  for (final model in models) {
    if (unsupportedIds.contains(model.id)) continue;
    final connection = idToConnection[model.id];
    if (connection == null) {
      other.add(model);
      continue;
    }
    final existing = byConnection[connection.id];
    if (existing == null) {
      byConnection[connection.id] = ModelInferenceConnectionGroup(
        inferenceConnectionId: connection.id,
        inferenceConnectionName: connection.name,
        models: [model],
      );
    } else {
      byConnection[connection.id] = ModelInferenceConnectionGroup(
        inferenceConnectionId: existing.inferenceConnectionId,
        inferenceConnectionName: existing.inferenceConnectionName,
        models: [...existing.models, model],
      );
    }
  }

  final groups = byConnection.values.toList();
  groups.sort((a, b) {
    final ai = inferenceConnections.indexWhere(
      (c) => c.id == a.inferenceConnectionId,
    );
    final bi = inferenceConnections.indexWhere(
      (c) => c.id == b.inferenceConnectionId,
    );
    return ai.compareTo(bi);
  });
  if (other.isNotEmpty) {
    groups.add(
      ModelInferenceConnectionGroup(
        inferenceConnectionName: kOtherInferenceConnectionGroupName,
        models: other,
      ),
    );
  }
  return groups;
}

/// Specs of each model id across [inferenceConnections]; ids without specs
/// are absent. The first connection listing an id wins, as in grouping.
Map<String, ModelSpecs> modelSpecsById(
  List<InferenceConnection> inferenceConnections,
) {
  final out = <String, ModelSpecs>{};
  for (final connection in inferenceConnections) {
    for (final m in connection.models) {
      final specs = m.specs;
      if (specs != null) out.putIfAbsent(m.id, () => specs);
    }
  }
  return out;
}

List<ModelInferenceConnectionGroup> filterModelGroups({
  required List<ModelInferenceConnectionGroup> groups,
  required String query,
}) {
  final q = query.trim().toLowerCase();
  if (q.isEmpty) return groups;
  final out = <ModelInferenceConnectionGroup>[];
  for (final g in groups) {
    final models = [
      for (final m in g.models)
        if (m.name.toLowerCase().contains(q) || m.id.toLowerCase().contains(q))
          m,
    ];
    if (models.isNotEmpty) {
      out.add(
        ModelInferenceConnectionGroup(
          inferenceConnectionId: g.inferenceConnectionId,
          inferenceConnectionName: g.inferenceConnectionName,
          models: models,
        ),
      );
    }
  }
  return out;
}
