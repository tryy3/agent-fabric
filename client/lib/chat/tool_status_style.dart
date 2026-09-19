import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';

enum ToolStatusVisual { completed, pending, failed }

ToolStatusVisual resolveToolStatusVisual({
  required String? status,
  required bool streaming,
}) {
  if (status == 'failed') return ToolStatusVisual.failed;
  if (status == 'completed') return ToolStatusVisual.completed;
  if (streaming ||
      status == null ||
      status == 'pending' ||
      status == 'in_progress') {
    return ToolStatusVisual.pending;
  }
  return ToolStatusVisual.completed;
}

Color toolFailedAmber(Brightness brightness) {
  return brightness == Brightness.dark
      ? const Color(0xFFFBBF24)
      : const Color(0xFFD97706);
}

Color toolStatusIconColor({
  required ToolStatusVisual visual,
  required ChatColors chat,
  required ColorScheme scheme,
  required Brightness brightness,
}) {
  return switch (visual) {
    ToolStatusVisual.failed => toolFailedAmber(brightness),
    ToolStatusVisual.pending => scheme.primary,
    ToolStatusVisual.completed => chat.stats.bar,
  };
}

Color toolStatusLabelColor({
  required ToolStatusVisual visual,
  required ColorScheme scheme,
  required Brightness brightness,
  required Color muted,
}) {
  return switch (visual) {
    ToolStatusVisual.failed => toolFailedAmber(brightness),
    ToolStatusVisual.pending => muted,
    ToolStatusVisual.completed => muted,
  };
}
