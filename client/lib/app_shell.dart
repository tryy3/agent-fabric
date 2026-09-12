import 'package:flutter/material.dart';

import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';

class AppShell extends StatefulWidget {
  const AppShell({super.key, required this.controller});

  final ChatController controller;

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int _selectedIndex = 0;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _selectedIndex,
            onDestinationSelected: (index) {
              setState(() => _selectedIndex = index);
            },
            labelType: NavigationRailLabelType.all,
            destinations: const [
              NavigationRailDestination(
                icon: Icon(Icons.chat),
                label: Text('Chat'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.settings),
                label: Text('Settings'),
              ),
            ],
          ),
          const VerticalDivider(thickness: 1, width: 1),
          Expanded(
            child: _selectedIndex == 0
                ? ChatScreen(controller: widget.controller)
                : const Center(child: Text('Settings')),
          ),
        ],
      ),
    );
  }
}
