import 'package:material_ui/material_ui.dart';

import '../chat/chat_controller.dart';

class ConnectivityBadge extends StatelessWidget {
  const ConnectivityBadge({super.key, required this.status});

  final ChatStatus status;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final (label, color) = switch (status) {
      ChatStatus.connected => ('Online', scheme.tertiary),
      ChatStatus.connecting ||
      ChatStatus.reconnecting => ('Reconnecting...', scheme.secondary),
      ChatStatus.disconnected || ChatStatus.error => ('Offline', scheme.error),
    };

    return Semantics(
      label: 'Connectivity: $label',
      child: Row(
        children: [
          Icon(Icons.circle, size: 10, color: color),
          const SizedBox(width: 6),
          Flexible(
            child: Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: Theme.of(context).textTheme.labelMedium,
            ),
          ),
        ],
      ),
    );
  }
}
