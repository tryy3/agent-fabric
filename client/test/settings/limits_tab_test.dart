import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/limits_fields.dart';
import 'package:agent_fabric_client/settings/limits_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class _FakeLimitsCatalog extends CatalogClient {
  _FakeLimitsCatalog(this.stored)
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  Map<String, dynamic> stored;
  Map<String, dynamic>? lastLimits;

  @override
  Future<PlaneSettings> getSettings() async => PlaneSettings(limits: stored);

  @override
  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
    Map<String, dynamic>? integrations,
    Object? webSearchIntegrationId = CatalogClient.fieldUnset,
    Object? fetchPageIntegrationId = CatalogClient.fieldUnset,
    String? platformInstructions,
    String? runtimeContext,
    String? gateInstructions,
    Map<String, dynamic>? permissions,
    Map<String, dynamic>? limits,
    Map<String, dynamic>? skills,
  }) async {
    lastLimits = limits;
    stored = limits ?? stored;
    return PlaneSettings(limits: stored);
  }
}

void main() {
  group('LimitsDraft', () {
    test('reads stored values and writes set fields only', () {
      final draft = LimitsDraft(
        json: {'defaultToolRounds': 12, 'maxTurnSeconds': 300},
      );
      expect(draft.defaultToolRounds.text, '12');
      expect(draft.maxToolRounds.text, '');
      expect(draft.toPlaneJson(), {
        'defaultToolRounds': 12,
        'maxTurnSeconds': 300,
      });
      expect(draft.toMergePatch(), {
        'defaultToolRounds': 12,
        'maxToolRounds': null,
        'maxTurnSeconds': 300,
      });
      draft.dispose();
    });

    test('validates ranges and the default against the maximum', () {
      final draft = LimitsDraft();
      expect(draft.error, isNull);
      draft.maxTurnSeconds.text = '3';
      expect(draft.secondsError, isNotNull);
      draft.maxTurnSeconds.text = '60';
      draft.defaultToolRounds.text = '30';
      draft.maxToolRounds.text = '20';
      expect(draft.error, contains('may not exceed'));
      draft.maxToolRounds.text = '40';
      expect(draft.error, isNull);
      draft.maxToolRounds.text = 'lots';
      expect(draft.maxRoundsError, isNotNull);
      draft.dispose();
    });
  });

  testWidgets('tab loads, edits and saves the plane limits', (tester) async {
    final catalog = _FakeLimitsCatalog({'maxToolRounds': 60});
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: LimitsTab(catalog: catalog)),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('limits-max-rounds')), findsOneWidget);
    expect(find.text('60'), findsOneWidget);
    FilledButton save() =>
        tester.widget<FilledButton>(find.byKey(const Key('limits-save')));
    expect(save().onPressed, isNull);

    await tester.enterText(
      find.byKey(const Key('limits-default-rounds')),
      '12',
    );
    await tester.pump();
    expect(save().onPressed, isNotNull);

    await tester.enterText(find.byKey(const Key('limits-max-seconds')), '3');
    await tester.pump();
    expect(save().onPressed, isNull);

    await tester.enterText(find.byKey(const Key('limits-max-seconds')), '600');
    await tester.pump();
    await tester.tap(find.byKey(const Key('limits-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastLimits, {
      'defaultToolRounds': 12,
      'maxToolRounds': 60,
      'maxTurnSeconds': 600,
    });
    expect(save().onPressed, isNull);
  });
}
