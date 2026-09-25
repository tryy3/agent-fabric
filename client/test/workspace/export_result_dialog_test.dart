import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/workspace/export_result_dialog.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('ExportResultDialog shows copy and open for each link', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ExportResultDialog(
            result: const ExportPublishResult(
              method: 'netlify',
              message: 'Published to Netlify',
              links: [
                ExportLink(
                  id: 'site',
                  label: 'Site',
                  url: 'https://demo.netlify.app',
                ),
                ExportLink(
                  id: 'deploy',
                  label: 'This deploy',
                  url: 'https://dep--demo.netlify.app',
                ),
              ],
            ),
          ),
        ),
      ),
    );

    expect(find.byKey(const Key('export-result-dialog')), findsOneWidget);
    expect(find.text('https://demo.netlify.app'), findsOneWidget);
    expect(find.byKey(const Key('export-copy-site')), findsOneWidget);
    expect(find.byKey(const Key('export-open-deploy')), findsOneWidget);
  });
}
