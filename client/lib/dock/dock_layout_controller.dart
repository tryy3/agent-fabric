import 'package:docking/docking.dart';
import 'package:flutter/widgets.dart';

import 'dock_ids.dart';

class DockItemWidgets {
  const DockItemWidgets({
    required this.threads,
    required this.files,
    required this.chat,
  });

  final Widget threads;
  final Widget files;
  final Widget chat;
}

class DockLayoutController extends ChangeNotifier {
  DockLayoutController();

  final DockingLayout layout = DockingLayout();
  dynamic focusedItemId;

  // Read by later toggle/open paths that rebuild cores from this set.
  // ignore: unused_field
  DockItemWidgets? _widgets;

  bool hasItem(dynamic id) => layout.findDockingItem(id) != null;

  DockingItem _core(String id, Widget child, {required double weight}) {
    return DockingItem(
      id: id,
      name: id,
      weight: weight,
      closable: true,
      keepAlive: id == DockIds.chat,
      widget: child,
    );
  }

  void resetToDefault({required DockItemWidgets widgets}) {
    _widgets = widgets;
    layout.root = DockingRow([
      _core(DockIds.threads, widgets.threads, weight: 0.18),
      _core(DockIds.files, widgets.files, weight: 0.16),
      _core(DockIds.chat, widgets.chat, weight: 0.66),
    ]);
    focusedItemId = DockIds.chat;
    notifyListeners();
  }
}
