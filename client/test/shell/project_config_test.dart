import 'package:agent_fabric_client/shell/project_config.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('reads tool and mcp names from project settings', () {
    final summary = ProjectConfigSummary.fromSettings({
      'tools': {
        'allow': ['read', ' write ', ''],
      },
      'mcp': {
        'servers': [
          {'name': 'github'},
          'local',
        ],
      },
      'environment': {'resourceId': 'res-1'},
    });

    expect(summary.tools, ['read', 'write']);
    expect(summary.mcpServers, ['github', 'local']);
    expect(summary.resourceId, 'res-1');
  });

  test('environment label prefers resolved resource kind and name', () {
    expect(
      environmentLabel(
        resolved: {
          'resource': {'kind': 'container', 'name': 'Ubuntu'},
        },
      ),
      'container · Ubuntu',
    );
    expect(environmentLabel(resourceName: 'Ubuntu'), 'Ubuntu');
    expect(environmentLabel(), 'Environment');
  });
}
