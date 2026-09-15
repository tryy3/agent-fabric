import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/appearance_tab.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() => SharedPreferences.setMockInitialValues({}));

  testWidgets('theme mode control updates AppearanceSettings', (tester) async {
    final appearance = await AppearanceSettings.load();
    await tester.pumpWidget(
      MaterialApp(home: AppearanceTab(settings: appearance)),
    );
    await tester.tap(find.byKey(const Key('theme-mode')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Dark').last);
    await tester.pumpAndSettle();
    expect(appearance.themeMode, ThemeMode.dark);
  });

  testWidgets('tapping a fill swatch overrides thinking fill', (tester) async {
    final appearance = await AppearanceSettings.load();
    await tester.pumpWidget(
      MaterialApp(
        theme: appearance.lightTheme,
        home: AppearanceTab(settings: appearance),
      ),
    );
    await tester.tap(find.byKey(const Key('swatch-thinking-fill-0')));
    await tester.pumpAndSettle();
    expect(
      appearance.hasOverride(Brightness.light, ChatColorRole.thinking),
      isTrue,
    );
  });

  testWidgets('Settings page shows Appearance tab', (tester) async {
    final display = await ChatDisplaySettings.load();
    final appearance = await AppearanceSettings.load();
    final catalog = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient(
        (_) async => http.Response(
          '[]',
          200,
          headers: {'content-type': 'application/json'},
        ),
      ),
    );
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: display,
          appearanceSettings: appearance,
        ),
      ),
    );
    expect(find.text('Appearance'), findsOneWidget);
  });
}
