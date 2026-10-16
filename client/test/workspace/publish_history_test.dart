import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/workspace/publish_history.dart';
import 'package:agent_fabric_client/workspace/publish_history_dialog.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

PublishRecord _publish(
  String id,
  String provider,
  int minute, {
  String target = '',
  String siteUrl = '',
  String? label,
  bool live = false,
  bool canDelete = false,
}) {
  return PublishRecord(
    id: id,
    provider: provider,
    providerLabel: label ?? provider,
    target: target,
    siteUrl: siteUrl,
    deployUrl: 'https://$id.example',
    live: live,
    canDelete: canDelete,
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
    canDelete: true,
  ),
  _publish(
    'pub_internal',
    'internal',
    2,
    siteUrl: 'https://pages.local/demo',
    label: 'Internal pages',
    live: true,
  ),
  _publish(
    'pub_new',
    'netlify',
    3,
    target: 'site_1',
    siteUrl: 'https://demo-renamed.netlify.app',
    label: 'Netlify',
    live: true,
  ),
];

/// Catalog double for the publish endpoints.
class _PublishCatalog extends CatalogClient {
  _PublishCatalog({
    List<PublishRecord>? publishes,
    this.loadError,
    this.syncResult,
    this.deleteError,
  }) : publishes = List.of(publishes ?? _history),
       super(
         baseUri: Uri.parse('http://catalog.test'),
         httpClient: MockClient((_) async => http.Response('{}', 200)),
       );

  List<PublishRecord> publishes;
  StateError? loadError;
  PublishSyncResult? syncResult;
  CatalogException? deleteError;
  int loadCalls = 0;
  int syncCalls = 0;
  final List<String> deleted = [];

  @override
  Future<List<PublishRecord>> listPublishes(
    String projectId, {
    String? provider,
    int? limit,
  }) async {
    loadCalls++;
    final error = loadError;
    if (error != null) {
      loadError = null;
      throw error;
    }
    return List.of(publishes);
  }

  @override
  Future<PublishSyncResult> syncPublishes(String projectId) async {
    syncCalls++;
    final result = syncResult!;
    publishes = List.of(result.publishes);
    return result;
  }

  @override
  Future<void> deletePublish(String projectId, String publishId) async {
    final error = deleteError;
    if (error != null) {
      throw error;
    }
    deleted.add(publishId);
    publishes.removeWhere((p) => p.id == publishId);
  }
}

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

  test('latestUrl follows the live deploy after a rollback', () {
    final groups = groupPublishes([
      _publish('a', 'netlify', 1, siteUrl: 'https://old.app', live: true),
      _publish('b', 'netlify', 2, siteUrl: 'https://new.app'),
    ]);
    expect(groups.single.latestUrl, 'https://old.app');
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

  test('syncNotices summarizes each provider', () {
    expect(
      syncNotices(const [
        PublishSyncProvider(
          provider: 'netlify',
          providerLabel: 'Netlify',
          added: 2,
          removed: 1,
        ),
        PublishSyncProvider(provider: 'internal', providerLabel: 'Internal'),
        PublishSyncProvider(
          provider: 'broken',
          providerLabel: 'Broken',
          error: 'provider down',
        ),
      ]),
      [
        (text: 'Netlify: 2 imported, 1 removed', isError: false),
        (text: 'Internal: up to date', isError: false),
        (text: 'Broken: provider down', isError: true),
      ],
    );
    expect(syncNotices(const []).single.isError, isFalse);
  });

  Future<void> pumpDialog(WidgetTester tester, CatalogClient catalog) async {
    tester.view.physicalSize = const Size(1200, 1600);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PublishHistoryDialog(
            catalog: catalog,
            projectId: 'proj_1',
            projectName: 'Demo',
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('PublishHistoryDialog lists latest and deploys per provider', (
    tester,
  ) async {
    await pumpDialog(tester, _PublishCatalog());

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
    expect(find.byKey(const Key('publish-live-pub_new')), findsOneWidget);
    expect(find.byKey(const Key('publish-live-pub_old')), findsNothing);
    // Only deletable deploys get a delete button; the live one never does.
    expect(find.byKey(const Key('publish-delete-pub_old')), findsOneWidget);
    expect(find.byKey(const Key('publish-delete-pub_new')), findsNothing);
    expect(find.byKey(const Key('publish-delete-pub_internal')), findsNothing);

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
      _PublishCatalog(
        publishes: _history.where((p) => p.provider == 'netlify').toList(),
      ),
    );
    expect(find.byKey(const Key('publish-filter-all')), findsNothing);
    expect(
      find.byKey(const Key('publish-group-netlify-site_1')),
      findsOneWidget,
    );
  });

  testWidgets('PublishHistoryDialog shows an empty state', (tester) async {
    await pumpDialog(tester, _PublishCatalog(publishes: const []));
    expect(find.byKey(const Key('publish-history-empty')), findsOneWidget);
  });

  testWidgets('PublishHistoryDialog retries after a load error', (
    tester,
  ) async {
    final catalog = _PublishCatalog(loadError: StateError('offline'));
    await pumpDialog(tester, catalog);
    expect(find.byKey(const Key('publish-history-retry')), findsOneWidget);

    await tester.tap(find.byKey(const Key('publish-history-retry')));
    await tester.pumpAndSettle();
    expect(catalog.loadCalls, 2);
    expect(find.byKey(const Key('publish-group-internal')), findsOneWidget);
  });

  testWidgets('Sync imports deploys and reports per provider', (tester) async {
    final imported = _publish(
      'pub_imported',
      'netlify',
      0,
      target: 'site_1',
      siteUrl: 'https://demo.netlify.app',
      label: 'Netlify',
      canDelete: true,
    );
    final catalog = _PublishCatalog(
      publishes: const [],
      syncResult: PublishSyncResult(
        providers: const [
          PublishSyncProvider(
            provider: 'netlify',
            providerLabel: 'Netlify',
            added: 1,
          ),
        ],
        publishes: [imported],
      ),
    );
    await pumpDialog(tester, catalog);
    expect(find.byKey(const Key('publish-history-empty')), findsOneWidget);

    await tester.tap(find.byKey(const Key('publish-history-sync')));
    await tester.pumpAndSettle();

    expect(catalog.syncCalls, 1);
    expect(find.text('Netlify: 1 imported'), findsOneWidget);
    expect(
      find.byKey(const Key('publish-deploy-pub_imported')),
      findsOneWidget,
    );
    expect(find.byKey(const Key('publish-history-empty')), findsNothing);
  });

  testWidgets('Delete asks first, then removes the deploy', (tester) async {
    final catalog = _PublishCatalog();
    await pumpDialog(tester, catalog);

    await tester.tap(find.byKey(const Key('publish-delete-pub_old')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('publish-delete-confirm')), findsOneWidget);
    await tester.tap(find.byKey(const Key('publish-delete-cancel')));
    await tester.pumpAndSettle();
    expect(catalog.deleted, isEmpty);
    expect(find.byKey(const Key('publish-deploy-pub_old')), findsOneWidget);

    await tester.tap(find.byKey(const Key('publish-delete-pub_old')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('publish-delete-confirm-button')));
    await tester.pumpAndSettle();

    expect(catalog.deleted, ['pub_old']);
    expect(find.byKey(const Key('publish-deploy-pub_old')), findsNothing);
    expect(find.textContaining('Deleted the'), findsOneWidget);
  });

  testWidgets('Delete failure keeps the deploy and shows the reason', (
    tester,
  ) async {
    final catalog = _PublishCatalog(
      deleteError: CatalogException(
        statusCode: 409,
        message: "the live deploy can't be deleted",
      ),
    );
    await pumpDialog(tester, catalog);

    await tester.tap(find.byKey(const Key('publish-delete-pub_old')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('publish-delete-confirm-button')));
    await tester.pumpAndSettle();

    expect(
      find.text("Delete failed: the live deploy can't be deleted"),
      findsOneWidget,
    );
    expect(find.byKey(const Key('publish-deploy-pub_old')), findsOneWidget);
  });
}
