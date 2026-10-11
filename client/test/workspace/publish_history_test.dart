import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/workspace/publish_history.dart';
import 'package:agent_fabric_client/workspace/publish_history_dialog.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

PublishRecord _publish(
  String id,
  String provider,
  int minute, {
  String target = '',
  String siteUrl = '',
  String? label,
}) {
  return PublishRecord(
    id: id,
    provider: provider,
    providerLabel: label ?? provider,
    target: target,
    siteUrl: siteUrl,
    deployUrl: 'https://$id.example',
    createdAt: DateTime.utc(2026, 10, 11, 12, minute),
  );
}

final _history = [
  _publish(
    'pub_old',
    'netlify',
    1,
    target: 'site_1',
    siteUrl: 'https://demo.netlify.app',
    label: 'Netlify',
  ),
  _publish(
    'pub_internal',
    'internal',
    2,
    siteUrl: 'https://pages.local/demo',
    label: 'Internal pages',
  ),
  _publish(
    'pub_new',
    'netlify',
    3,
    target: 'site_1',
    siteUrl: 'https://demo-renamed.netlify.app',
    label: 'Netlify',
  ),
];

void main() {
  test('groupPublishes groups by provider and target, newest first', () {
    final groups = groupPublishes(_history);
    expect(groups.map((g) => g.provider), ['netlify', 'internal']);
    final netlify = groups.first;
    expect(netlify.publishes.map((p) => p.id), ['pub_new', 'pub_old']);
    expect(netlify.latestUrl, 'https://demo-renamed.netlify.app');
    expect(netlify.showTarget, isFalse);
    expect(netlify.lastPublishedAt, DateTime.utc(2026, 10, 11, 12, 3));
  });

  test('groupPublishes shows the target when a provider has several', () {
    final groups = groupPublishes([
      ..._history,
      _publish('pub_other', 'netlify', 4, target: 'site_2', label: 'Netlify'),
    ]);
    expect(groups.map((g) => (g.provider, g.target)), [
      ('netlify', 'site_2'),
      ('netlify', 'site_1'),
      ('internal', ''),
    ]);
    expect(
      groups.where((g) => g.provider == 'netlify').every((g) => g.showTarget),
      isTrue,
    );
    expect(groups.last.showTarget, isFalse);
    // No site URL on the only publish: no latest link to show.
    expect(groups.first.latestUrl, isEmpty);
  });

  test('publishProviders counts per provider, most recent first', () {
    final providers = publishProviders(_history);
    expect(providers.map((p) => (p.provider, p.label, p.count)), [
      ('netlify', 'Netlify', 2),
      ('internal', 'Internal pages', 1),
    ]);
  });

  Future<void> pumpDialog(
    WidgetTester tester,
    Future<List<PublishRecord>> Function() load,
  ) async {
    tester.view.physicalSize = const Size(1200, 1600);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PublishHistoryDialog(projectName: 'Demo', load: load),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('PublishHistoryDialog lists latest and deploys per provider', (
    tester,
  ) async {
    await pumpDialog(tester, () async => _history);

    expect(find.text('Published · Demo'), findsOneWidget);
    expect(
      find.byKey(const Key('publish-group-netlify-site_1')),
      findsOneWidget,
    );
    expect(find.byKey(const Key('publish-group-internal')), findsOneWidget);
    expect(find.text('https://demo-renamed.netlify.app'), findsOneWidget);
    expect(
      find.byKey(const Key('publish-latest-open-netlify-site_1')),
      findsOneWidget,
    );
    for (final id in ['pub_old', 'pub_internal', 'pub_new']) {
      expect(find.byKey(Key('publish-deploy-$id')), findsOneWidget);
      expect(find.text('https://$id.example'), findsOneWidget);
    }
    expect(find.text('Current'), findsNWidgets(2));

    await tester.tap(find.byKey(const Key('publish-filter-internal')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('publish-group-netlify-site_1')), findsNothing);
    expect(find.byKey(const Key('publish-group-internal')), findsOneWidget);

    await tester.tap(find.byKey(const Key('publish-filter-all')));
    await tester.pumpAndSettle();
    expect(
      find.byKey(const Key('publish-group-netlify-site_1')),
      findsOneWidget,
    );
  });

  testWidgets('PublishHistoryDialog hides filters for a single provider', (
    tester,
  ) async {
    await pumpDialog(
      tester,
      () async => _history.where((p) => p.provider == 'netlify').toList(),
    );
    expect(find.byKey(const Key('publish-filter-all')), findsNothing);
    expect(
      find.byKey(const Key('publish-group-netlify-site_1')),
      findsOneWidget,
    );
  });

  testWidgets('PublishHistoryDialog shows an empty state', (tester) async {
    await pumpDialog(tester, () async => const []);
    expect(find.byKey(const Key('publish-history-empty')), findsOneWidget);
  });

  testWidgets('PublishHistoryDialog retries after a load error', (
    tester,
  ) async {
    var calls = 0;
    await pumpDialog(tester, () async {
      calls++;
      if (calls == 1) {
        throw StateError('offline');
      }
      return _history;
    });
    expect(find.byKey(const Key('publish-history-retry')), findsOneWidget);

    await tester.tap(find.byKey(const Key('publish-history-retry')));
    await tester.pumpAndSettle();
    expect(calls, 2);
    expect(find.byKey(const Key('publish-group-internal')), findsOneWidget);
  });
}
