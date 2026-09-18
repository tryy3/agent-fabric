import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('resolve null uses pretty default', () {
    final m = resolveViewMode(null);
    expect(m.id, 'pretty');
    expect(m.markdownRender, isTrue);
    expect(m.thinkingVisibility, VisibilityMode.collapsed);
    expect(m.toolVisibility, VisibilityMode.collapsed);
    expect(m.toolIO, ToolIOMode.both);
    expect(m.rawRequests, isFalse);
  });

  test('resolve detailed', () {
    final m = resolveViewMode('detailed');
    expect(m.id, 'detailed');
    expect(m.markdownRender, isFalse);
  });

  test('unknown id falls back to default', () {
    final m = resolveViewMode('nope');
    expect(m.id, 'pretty');
  });

  test('registry has pretty and detailed labels', () {
    expect(kBuiltInViewModes.map((m) => m.label).toList(), [
      'Pretty',
      'Detailed',
    ]);
  });
}
