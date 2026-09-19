import 'package:agent_fabric_client/chat/tool_status_style.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  test('maps failed / completed / pending statuses', () {
    expect(
      resolveToolStatusVisual(status: 'failed', streaming: false),
      ToolStatusVisual.failed,
    );
    expect(
      resolveToolStatusVisual(status: 'completed', streaming: false),
      ToolStatusVisual.completed,
    );
    expect(
      resolveToolStatusVisual(status: 'pending', streaming: false),
      ToolStatusVisual.pending,
    );
    expect(
      resolveToolStatusVisual(status: 'in_progress', streaming: false),
      ToolStatusVisual.pending,
    );
    expect(
      resolveToolStatusVisual(status: null, streaming: true),
      ToolStatusVisual.pending,
    );
    expect(
      resolveToolStatusVisual(status: 'unknown', streaming: false),
      ToolStatusVisual.completed,
    );
  });

  test('icon and label colors follow visual', () {
    final chat = ChatColors.light();
    const scheme = ColorScheme.light();
    expect(
      toolStatusIconColor(
        visual: ToolStatusVisual.completed,
        chat: chat,
        scheme: scheme,
        brightness: Brightness.light,
      ),
      chat.stats.bar,
    );
    expect(
      toolStatusIconColor(
        visual: ToolStatusVisual.pending,
        chat: chat,
        scheme: scheme,
        brightness: Brightness.light,
      ),
      scheme.primary,
    );
    expect(
      toolStatusIconColor(
        visual: ToolStatusVisual.failed,
        chat: chat,
        scheme: scheme,
        brightness: Brightness.light,
      ),
      toolFailedAmber(Brightness.light),
    );
    expect(
      toolStatusLabelColor(
        visual: ToolStatusVisual.failed,
        scheme: scheme,
        brightness: Brightness.light,
        muted: const Color(0xFF888888),
      ),
      toolFailedAmber(Brightness.light),
    );
    expect(
      toolStatusLabelColor(
        visual: ToolStatusVisual.completed,
        scheme: scheme,
        brightness: Brightness.light,
        muted: const Color(0xFF888888),
      ),
      const Color(0xFF888888),
    );
  });
}
