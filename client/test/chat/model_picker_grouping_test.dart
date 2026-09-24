import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/model_picker_grouping.dart';
import 'package:flutter_test/flutter_test.dart';

Provider _provider({
  required String id,
  required String name,
  required List<ModelInfo> models,
}) {
  final now = DateTime.utc(2026, 9, 18);
  return Provider(
    id: id,
    name: name,
    type: 'openai_compatible',
    baseUrl: 'http://example',
    apiKey: 'k',
    models: models,
    createdAt: now,
    updatedAt: now,
  );
}

void main() {
  test(
    'groups matched models under provider name with count order preserved',
    () {
      final models = const [
        ModelOption(id: 'm1', name: 'One'),
        ModelOption(id: 'm2', name: 'Two'),
        ModelOption(id: 'orphan', name: 'Orphan'),
      ];
      final providers = [
        _provider(
          id: 'p1',
          name: 'Local',
          models: const [
            ModelInfo(id: 'm1', name: 'One'),
            ModelInfo(id: 'm2', name: 'Two'),
          ],
        ),
      ];

      final groups = groupModelsByProvider(
        models: models,
        providers: providers,
      );

      expect(groups, hasLength(2));
      expect(groups[0].providerId, 'p1');
      expect(groups[0].providerName, 'Local');
      expect(groups[0].models.map((m) => m.id), ['m1', 'm2']);
      expect(groups[1].providerId, isNull);
      expect(groups[1].providerName, kOtherProviderGroupName);
      expect(groups[1].models.single.id, 'orphan');
    },
  );

  test('empty providers puts everything in Other', () {
    final groups = groupModelsByProvider(
      models: const [ModelOption(id: 'm1', name: 'One')],
      providers: const [],
    );
    expect(groups, hasLength(1));
    expect(groups.single.providerName, kOtherProviderGroupName);
  });

  test('first provider wins when id appears twice', () {
    final models = const [ModelOption(id: 'm1', name: 'One')];
    final providers = [
      _provider(
        id: 'p1',
        name: 'First',
        models: const [ModelInfo(id: 'm1', name: 'One')],
      ),
      _provider(
        id: 'p2',
        name: 'Second',
        models: const [ModelInfo(id: 'm1', name: 'One')],
      ),
    ];
    final groups = groupModelsByProvider(models: models, providers: providers);
    expect(groups.single.providerName, 'First');
  });

  test(
    'filter hides empty groups and matches name or id case-insensitively',
    () {
      final groups = [
        const ModelProviderGroup(
          providerId: 'p1',
          providerName: 'Local',
          models: [
            ModelOption(id: 'alpha-id', name: 'Alpha'),
            ModelOption(id: 'beta-id', name: 'Beta'),
          ],
        ),
        const ModelProviderGroup(
          providerName: kOtherProviderGroupName,
          models: [ModelOption(id: 'zz', name: 'Zed')],
        ),
      ];

      final byName = filterModelGroups(groups: groups, query: 'alp');
      expect(byName, hasLength(1));
      expect(byName.single.models.single.name, 'Alpha');

      final byId = filterModelGroups(groups: groups, query: 'BETA-ID');
      expect(byId.single.models.single.id, 'beta-id');

      final none = filterModelGroups(groups: groups, query: 'nope');
      expect(none, isEmpty);
    },
  );

  test('blank query returns groups unchanged', () {
    final groups = [
      const ModelProviderGroup(
        providerName: 'Local',
        models: [ModelOption(id: 'm1', name: 'One')],
      ),
    ];
    expect(filterModelGroups(groups: groups, query: '  '), groups);
  });
}
