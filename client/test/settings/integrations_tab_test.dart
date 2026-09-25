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
    expect(find.byKey(const Key('netlify-api-key')), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('netlify-api-key')),
      'nlt_test_token',
    );
    await tester.enterText(
      find.byKey(const Key('netlify-account-id')),
      'acct_test',
    );
    await tester.tap(find.byKey(const Key('integrations-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastIntegrations?['netlify'], isA<Map>());
    final netlify = catalog.lastIntegrations!['netlify'] as Map;
    expect(netlify['apiKey'], 'nlt_test_token');
    expect(netlify['accountId'], 'acct_test');
  });
}

class _FakeIntegrationsCatalog extends CatalogClient {
  _FakeIntegrationsCatalog() : super(baseUri: Uri.parse('http://catalog.test'));

  Map<String, dynamic> integrations = {
    'netlify': {'apiKey': '', 'accountId': ''},
  };
  Map<String, dynamic>? lastIntegrations;

  @override
  Future<PlaneSettings> getSettings() async {
    return PlaneSettings(integrations: Map<String, dynamic>.from(integrations));
  }

  @override
  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
    Map<String, dynamic>? integrations,
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
