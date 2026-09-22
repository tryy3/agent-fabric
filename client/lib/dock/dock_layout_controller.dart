import 'package:docking/docking.dart';
import 'package:flutter/widgets.dart';

import '../workspace/open_with.dart';
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

  /// Tabs [view] onto the focused item, or splits it to the right when [toSide].
  ///
  /// An id that is already open is focused and left in place.
  void openDocument({
    required OpenView view,
    required Widget child,
    bool toSide = false,
  }) {
    final id = DockIds.doc(view.path, view.appId);
    if (hasItem(id)) {
      focusedItemId = id;
      notifyListeners();
      return;
    }
    final item = DockingItem(
      id: id,
      name: view.tabLabel,
      closable: true,
      keepAlive: true,
      widget: child,
    );
    final target = _resolveFocusItem();
    focusedItemId = id;
    if (target == null) {
      layout.root = item;
      return;
    }
    final dropTarget = _dropTarget(target);
    if (toSide) {
      layout.addItemOn(
        newItem: item,
        targetArea: dropTarget,
        dropPosition: DropPosition.right,
      );
      return;
    }
    layout.addItemOn(
      newItem: item,
      targetArea: dropTarget,
      dropIndex: dropTarget is DockingTabs ? dropTarget.childrenCount : 1,
    );
  }

  /// Removes one document item. A focused document falls back to chat, then any item.
  void closeDocument(String dockId) {
    if (!DockIds.isDoc(dockId) || !hasItem(dockId)) return;
    if (focusedItemId == dockId) {
      focusedItemId = _fallbackFocusId(skip: {dockId});
    }
    layout.removeItemByIds([dockId]);
  }

  /// Removes every document item and leaves cores in place.
  void clearDocuments() {
    final ids = <dynamic>[
      for (final area in layout.layoutAreas())
        if (area is DockingItem && DockIds.isDoc(area.id)) area.id,
    ];
    if (ids.isEmpty) return;
    if (ids.contains(focusedItemId)) {
      focusedItemId = _fallbackFocusId(skip: ids.toSet());
    }
    layout.removeItemByIds(ids);
  }

  /// Focused item, or chat, or any remaining item when focus points at a removed id.
  DockingItem? _resolveFocusItem() {
    final focused = layout.findDockingItem(focusedItemId);
    if (focused != null) return focused;
    final chat = layout.findDockingItem(DockIds.chat);
    if (chat != null) return chat;
    for (final area in layout.layoutAreas()) {
      if (area is DockingItem) return area;
    }
    return null;
  }

  dynamic _fallbackFocusId({required Set<dynamic> skip}) {
    if (!skip.contains(DockIds.chat) && hasItem(DockIds.chat)) {
      return DockIds.chat;
    }
    for (final area in layout.layoutAreas()) {
      if (area is DockingItem && !skip.contains(area.id)) return area.id;
    }
    return null;
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
          targetArea: _dropTarget(threads),
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
      targetArea: _dropTarget(target),
      dropPosition: position,
    );
  }

  /// [addItemOn] on a tab child throws because nested tabbed panels are
  /// forbidden. Drop beside the [DockingTabs] that owns the item.
  DropArea _dropTarget(DropArea area) {
    if (area is DockingItem && area.parent is DockingTabs) {
      return area.parent! as DockingTabs;
    }
    return area;
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
