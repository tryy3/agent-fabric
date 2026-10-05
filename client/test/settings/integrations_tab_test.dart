import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/integrations_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('IntegrationsTab loads and saves Netlify credentials', (
    tester,
  ) async {
    final catalog = _FakeIntegrationsCatalog();
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: IntegrationsTab(catalog: catalog)),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('integrations-tab')), findsOneWidget);
    expect(find.byKey(const Key('tool-integration-add')), findsOneWidget);

    await tester.scrollUntilVisible(
      find.byKey(const Key('netlify-api-key')),
      300,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.enterText(
      find.byKey(const Key('netlify-api-key')),
      'nlt_test_token',
    );
    await tester.enterText(
      find.byKey(const Key('netlify-account-id')),
      'acct_test',
    );
    await tester.scrollUntilVisible(
      find.text('Save Netlify'),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.tap(find.text('Save Netlify'));
    await tester.pumpAndSettle();

    expect(catalog.lastIntegrations?['netlify'], isA<Map>());
    final netlify = catalog.lastIntegrations!['netlify'] as Map;
    expect(netlify['apiKey'], 'nlt_test_token');
    expect(netlify['accountId'], 'acct_test');
  });

  testWidgets('IntegrationsTab shows write-only secret configured state', (
    tester,
  ) async {
    final catalog = _FakeIntegrationsCatalog()
      ..tools = [
        const ToolIntegration(
          id: 'ti_1',
          name: 'Linkup',
          kind: 'linkup',
          enabled: true,
          scope: 'plane',
          endpoint: 'https://mcp.linkup.so/mcp',
          mode: 'external',
          capabilities: ['web_search', 'fetch_page'],
          secretsConfigured: {'apiKey': true},
          healthStatus: 'unknown',
        ),
      ];
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: IntegrationsTab(catalog: catalog)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.textContaining('apiKey configured'), findsOneWidget);
    expect(
      find.byKey(const Key('tool-integration-ti_1-cap-web_search')),
      findsOneWidget,
    );
    expect(
      find.byKey(const Key('tool-integration-ti_1-cap-fetch_page')),
      findsOneWidget,
    );
    expect(find.text('Search'), findsOneWidget);
    expect(find.text('Fetch'), findsOneWidget);
  });
}

class _FakeIntegrationsCatalog extends CatalogClient {
  _FakeIntegrationsCatalog() : super(baseUri: Uri.parse('http://catalog.test'));

  Map<String, dynamic> integrations = {
    'netlify': {'apiKey': '', 'accountId': ''},
  };
  Map<String, dynamic>? lastIntegrations;
  List<ToolIntegration> tools = const [];

  @override
  Future<PlaneSettings> getSettings() async {
    return PlaneSettings(integrations: Map<String, dynamic>.from(integrations));
  }

  @override
  Future<List<ToolIntegration>> listToolIntegrations() async => tools;

  @override
  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
    Map<String, dynamic>? integrations,
    Object? webSearchIntegrationId = CatalogClient.fieldUnset,
    Object? fetchPageIntegrationId = CatalogClient.fieldUnset,
    String? platformInstructions,
    String? runtimeContext,
    Map<String, dynamic>? permissions,
  }) async {
    lastIntegrations = integrations;
    if (integrations != null) {
      this.integrations = Map<String, dynamic>.from(integrations);
    }
    return PlaneSettings(
      integrations: Map<String, dynamic>.from(this.integrations),
    );
  }
}
