import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/model_specs_tab.dart';
import 'package:agent_fabric_client/ui/model_specs_widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class _FakeCatalog extends CatalogClient {
  _FakeCatalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('', 404)),
      );

  int syncs = 0;
  String? savedSource;

  @override
  Future<ModelSpecsStatus> getModelSpecsStatus() async => ModelSpecsStatus(
    sourceUrl: '',
    effectiveSourceUrl: 'https://models.dev/api.json',
    syncIntervalHours: 24,
    enabled: true,
    providerCount: 1,
    modelCount: 1,
    lastSyncedAt: DateTime.now().subtract(const Duration(hours: 3)),
  );

  @override
  Future<List<SpecsProviderSummary>> listSpecsProviders() async => const [
    SpecsProviderSummary(
      ref: SpecsProviderRef(id: 'acme', name: 'Acme', logoUrl: '/logo'),
      modelCount: 1,
    ),
  ];

  @override
  Future<SpecsProviderDetail> getSpecsProvider(String id) async =>
      const SpecsProviderDetail(
        ref: SpecsProviderRef(id: 'acme', name: 'Acme', logoUrl: '/logo'),
        models: [
          ModelInfo(
            id: 'big-1',
            name: 'Big 1',
            specs: ModelSpecs(
              toolCall: true,
              reasoning: true,
              contextLimit: 200000,
              costInput: 3,
              costOutput: 15,
            ),
          ),
        ],
      );

  @override
  Future<ProviderLogoImage?> providerLogo(String logoUrl) async => null;

  @override
  Future<void> updateModelSpecsSettings({
    String? sourceUrl,
    int? syncIntervalHours,
    bool? enabled,
  }) async => savedSource = sourceUrl;

  @override
  Future<ModelSpecsStatus> syncModelSpecs() async {
    syncs++;
    return getModelSpecsStatus();
  }
}

void main() {
  testWidgets('shows last sync, syncs now, saves source', (tester) async {
    final catalog = _FakeCatalog();
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: ModelSpecsTab(catalog: catalog)),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('last synced 3 h ago'), findsOneWidget);

    await tester.tap(find.byKey(const Key('model-specs-source-section')));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('model-specs-source')),
      'http://my.host/api.json',
    );
    await tester.tap(find.byKey(const Key('model-specs-sync')));
    await tester.pumpAndSettle();
    expect(catalog.syncs, 1);
    expect(catalog.savedSource, 'http://my.host/api.json');
  });

  test('formatters', () {
    expect(formatTokenCount(200000), '200K');
    expect(formatTokenCount(1048576), '1.0M');
    expect(formatTokenCount(1000000), '1M');
    expect(formatPrice(0), 'free');
    expect(formatPrice(0.3), r'$0.30');
    expect(formatPrice(15), r'$15');
  });
}
