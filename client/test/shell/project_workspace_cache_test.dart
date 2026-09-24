import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:agent_fabric_client/shell/project_workspace_session.dart';
import 'package:agent_fabric_client/workspace/workspace_controller.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/testing.dart';

void main() {
  test('store put get remove and retain dispose sessions', () {
    final catalog = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient((_) async => throw StateError('unused')),
    );
    DockItemWidgets items(WorkspaceController w) => const DockItemWidgets(
      threads: SizedBox(),
      files: SizedBox(),
      chat: SizedBox(),
    );
    final store = ProjectSessionStore();
    final a = createProjectSession(
      projectId: 'proj-a',
      catalog: catalog,
      itemWidgetsFor: items,
    );
    final b = createProjectSession(
      projectId: 'proj-b',
      catalog: catalog,
      itemWidgetsFor: items,
    );
    store.put(a);
    store.put(b);
    expect(store.length, 2);
    expect(store['proj-a'], same(a));

    expect(store.remove('proj-a'), same(a));
    expect(store.length, 1);
    a.dispose();

    store.retain({'proj-x'});
    expect(store.length, 0);
  });
}
