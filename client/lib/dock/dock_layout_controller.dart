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
  DockLayoutController() {
    layout.addListener(notifyListeners);
  }

  final DockingLayout layout = DockingLayout();
  dynamic focusedItemId;

  DockItemWidgets? _widgets;

  bool hasItem(dynamic id) => layout.findDockingItem(id) != null;

  /// Removes the core when it is open. Inserts and focuses it when it is hidden.
  void toggleCore(String coreId) {
    if (!_isCore(coreId)) return;
    if (hasItem(coreId)) {
      layout.removeItem(item: layout.findDockingItem(coreId)!);
      return;
    }
    _openCore(coreId);
  }

  /// Focuses the core. Inserts it first when it is hidden.
  void ensureCore(String coreId) {
    if (!_isCore(coreId)) return;
    if (hasItem(coreId)) {
      focusedItemId = coreId;
      notifyListeners();
      return;
    }
    _openCore(coreId);
  }

  bool _openCore(String coreId) {
    final item = _coreItem(coreId);
    if (item == null) return false;
    focusedItemId = coreId;
    _insertCore(coreId, item);
    return true;
  }

  DockingItem? _coreItem(String coreId) {
    final widgets = _widgets;
    if (widgets == null) return null;
    switch (coreId) {
      case DockIds.threads:
        return _core(coreId, widgets.threads, weight: 0.18);
      case DockIds.files:
        return _core(coreId, widgets.files, weight: 0.16);
      case DockIds.chat:
        return _core(coreId, widgets.chat, weight: 0.66);
    }
    return null;
  }

  void _insertCore(String coreId, DockingItem item) {
    if (coreId == DockIds.files) {
      final threads = layout.findDockingItem(DockIds.threads);
      if (threads != null) {
        layout.addItemOn(
          newItem: item,
          targetArea: threads,
          dropPosition: DropPosition.right,
        );
        return;
      }
    }
    _addAtRootEdge(item, left: coreId != DockIds.chat);
  }

  /// Threads (and files without threads) go on the left. Chat goes on the right.
  ///
  /// [addItemOnRoot] only accepts a [DropArea] root. A row or column is not
  /// one, so the leftmost or rightmost leaf [DropArea] is the drop target.
  /// Assigning `layout.root` to a new row that still holds the live tree
  /// disposes those areas and throws.
  void _addAtRootEdge(DockingItem item, {required bool left}) {
    final root = layout.root;
    final position = left ? DropPosition.left : DropPosition.right;
    if (root == null) {
      layout.root = item;
      return;
    }
    if (root is DropArea) {
      layout.addItemOnRoot(newItem: item, dropPosition: position);
      return;
    }
    final target = _edgeDropArea(root, left: left);
    if (target == null) return;
    layout.addItemOn(
      newItem: item,
      targetArea: target,
      dropPosition: position,
    );
  }

  /// Leftmost or rightmost [DropArea] under [area].
  DropArea? _edgeDropArea(DockingArea area, {required bool left}) {
    if (area is DropArea) return area as DropArea;
    if (area is! DockingParentArea || area.childrenCount == 0) return null;
    final index = left ? 0 : area.childrenCount - 1;
    return _edgeDropArea(area.childAt(index), left: left);
  }

  bool _isCore(String coreId) =>
      coreId == DockIds.threads ||
      coreId == DockIds.files ||
      coreId == DockIds.chat;

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
    focusedItemId = DockIds.chat;
    layout.root = DockingRow([
      _core(DockIds.threads, widgets.threads, weight: 0.18),
      _core(DockIds.files, widgets.files, weight: 0.16),
      _core(DockIds.chat, widgets.chat, weight: 0.66),
    ]);
  }

  @override
  void dispose() {
    layout.removeListener(notifyListeners);
    super.dispose();
  }
}
