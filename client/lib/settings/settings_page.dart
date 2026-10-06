import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../chat/display_settings.dart';
import 'assistants_tab.dart';
import 'appearance_settings.dart';
import 'display_tab.dart';
import 'environment_tab.dart';
import 'instructions_tab.dart';
import 'integrations_tab.dart';
import 'projects_tab.dart';
import 'inference_connections_tab.dart';
import 'model_specs_tab.dart';
import 'permissions_tab.dart';
import 'resources_tab.dart';

class SettingsPage extends StatelessWidget {
  const SettingsPage({
    super.key,
    required this.catalog,
    required this.displaySettings,
    required this.appearanceSettings,
  });

  final CatalogClient catalog;
  final ChatDisplaySettings displaySettings;
  final AppearanceSettings appearanceSettings;

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 10,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Settings'),
          bottom: const TabBar(
            isScrollable: true,
            tabs: [
              Tab(text: 'Connections'),
              Tab(text: 'Model specs'),
              Tab(text: 'Assistants'),
              Tab(text: 'Instructions'),
              Tab(text: 'Projects'),
              Tab(text: 'Resources'),
              Tab(text: 'Environment'),
              Tab(text: 'Permissions'),
              Tab(text: 'Integrations'),
              Tab(text: 'Display'),
            ],
          ),
        ),
        body: TabBarView(
          children: [
            InferenceConnectionsTab(catalog: catalog),
            ModelSpecsTab(catalog: catalog),
            AssistantsTab(catalog: catalog),
            InstructionsTab(catalog: catalog),
            ProjectsTab(catalog: catalog),
            ResourcesTab(catalog: catalog),
            EnvironmentTab(catalog: catalog),
            PermissionsTab(catalog: catalog),
            IntegrationsTab(catalog: catalog),
            DisplayTab(
              displaySettings: displaySettings,
              appearanceSettings: appearanceSettings,
            ),
          ],
        ),
      ),
    );
  }
}
