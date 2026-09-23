import 'dart:async';

import 'package:docking/docking.dart';
import 'package:flutter/widgets.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../workspace/open_with.dart';
import 'dock_chat_tab_status.dart';
import 'dock_ids.dart';
import 'dock_tab_icons.dart';

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
    layout.addListener(_onLayoutChanged);
  }

  static const prefsKey = 'dock_shell_layout_v1';
  static const _persistDelay = Duration(milliseconds: 300);

  final DockingLayout layout = DockingLayout();
  dynamic focusedItemId;

  DockItemWidgets? _widgets;
  Timer? _persistTimer;
  bool _disposed = false;

  void _onLayoutChanged() {
    notifyListeners();
    schedulePersist();
  }

  /// Debounces [persist] so layout notifications stay synchronous.
  void schedulePersist() {
    if (_disposed) return;
    _persistTimer?.cancel();
    _persistTimer = Timer(_persistDelay, () {
      _persistTimer = null;
      if (_disposed) return;
      unawaited(_persistQuietly());
    });
  }

  Future<void> _persistQuietly() async {
    try {
      await persist();
    } catch (_) {
      // Unit tests that never mock preferences, and a missing plugin, skip the write.
    }
  }

  /// Saves the current layout string. Document ids may be present; [restore] drops them.
  Future<void> persist() async {
    final encoded = layout.stringify(parser: _ShellLayoutCodec(_widgets));
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(prefsKey, encoded);
  }

  /// Rebuilds cores from [prefsKey]. Strips every `doc:` id after a successful load.
  ///
  /// Missing, empty, or unreadable preferences fall back to [resetToDefault].
  Future<void> restore({required DockItemWidgets widgets}) async {
    _widgets = widgets;
    try {
      final prefs = await SharedPreferences.getInstance();
      final saved = prefs.getString(prefsKey);
      if (saved == null || saved.isEmpty) {
        resetToDefault(widgets: widgets);
        return;
      }
      final codec = _ShellLayoutCodec(widgets);
      layout.load(layout: saved, parser: codec, builder: codec);
      _stripDocumentsPreservingWeights();
      _seedFocusAfterRestore();
    } catch (_) {
      resetToDefault(widgets: widgets);
    }
  }

  /// Saved layouts do not store [focusedItemId]. Seed it once so a visible
  /// chat pane is focused, and an unselected chat tab stays unfocused.
  void _seedFocusAfterRestore() {
    if (focusedItemId != null || !hasItem(DockIds.chat)) return;
    final tabs = layout.findDockingTabsWithItem(DockIds.chat);
    if (tabs == null) {
      focusedItemId = DockIds.chat;
      return;
    }
    if (tabs.childrenCount == 0) return;
    final index = tabs.selectedIndex.clamp(0, tabs.childrenCount - 1);
    focusedItemId = tabs.childAt(index).id;
  }

  bool hasItem(dynamic id) => layout.findDockingItem(id) != null;

  /// Removes the core when it is open. Inserts and focuses it when it is hidden.
  void toggleCore(String coreId) {
    if (!_isCore(coreId)) return;
    if (hasItem(coreId)) {
      if (focusedItemId == coreId) {
        focusedItemId = _focusAfterClose(coreId);
      }
      layout.removeItem(item: layout.findDockingItem(coreId)!);
      return;
    }
    _openCore(coreId);
  }

  /// Focuses the core. Inserts it first when it is hidden.
  ///
  /// An already-open core is selected in its tab group before focus changes.
  void ensureCore(String coreId) {
    if (!_isCore(coreId)) return;
    if (hasItem(coreId)) {
      _selectDocumentTab(coreId);
      focusedItemId = coreId;
      layout.rebuild();
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
      _selectDocumentTab(id);
      layout.rebuild();
      return;
    }
    final item = DockingItem(
      id: id,
      name: view.tabLabel,
      closable: true,
      keepAlive: true,
      leading: dockTabLeadingForApp(view.appId),
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
    } else {
      layout.addItemOn(
        newItem: item,
        targetArea: dropTarget,
        dropIndex: dropTarget is DockingTabs ? dropTarget.childrenCount : 1,
      );
    }
    _selectDocumentTab(id);
  }

  /// Points the tab group at [id] when that item is tabbed.
  ///
  /// A brand-new side split is its own pane, so there is nothing to select.
  void _selectDocumentTab(dynamic id) {
    final tabs = layout.findDockingTabsWithItem(id);
    if (tabs == null) return;
    for (var i = 0; i < tabs.childrenCount; i++) {
      if (tabs.childAt(i).id == id) {
        tabs.selectedIndex = i;
        return;
      }
    }
  }

  /// Removes one document item. A focused document falls back to chat, then any item.
  void closeDocument(String dockId) {
    if (!DockIds.isDoc(dockId) || !hasItem(dockId)) return;
    if (focusedItemId == dockId) {
      focusedItemId = _fallbackFocusId(skip: {dockId});
    }
    layout.removeItemByIds([dockId]);
  }

  /// Replaces the chat tab leading and asks the layout to rebuild.
  void setChatTabLead(DockChatTabLead lead) {
    final item = layout.findDockingItem(DockIds.chat);
    if (item == null) return;
    item.leading = dockTabLeadingForId(DockIds.chat, chatLead: lead);
    layout.rebuild();
  }

  /// Shows the dirty close button, or restores the package close control.
  void setDocumentDirtyClose(
    String dockId, {
    required bool dirty,
    required VoidCallback onClose,
  }) {
    final item = layout.findDockingItem(dockId);
    if (item == null) return;
    if (dirty) {
      item.closable = false;
      item.buttons = [dirtyCloseTabButton(onClose: onClose)];
    } else {
      item.closable = true;
      item.buttons = const [];
    }
    layout.rebuild();
  }

  /// Drops `doc:` items loaded by [restore] and puts each removed parent's
  /// split weight on the area that replaces it.
  ///
  /// [DockingLayout.removeItemByIds] rebuilds rows, columns, and tabs without
  /// copying `weight`. Child-first capture lets an outer collapsed parent
  /// overwrite an inner one that promotes the same survivor. Sibling groups
  /// that stay in place get their weight applied to the rebuilt area with the
  /// same non-doc leaves.
  void _stripDocumentsPreservingWeights() {
    final docIds = <dynamic>[
      for (final area in layout.layoutAreas())
        if (area is DockingItem && DockIds.isDoc(area.id)) area.id,
    ];
    if (docIds.isEmpty) return;
    final docIdSet = docIds.toSet();
    if (docIdSet.contains(focusedItemId)) {
      focusedItemId = _fallbackFocusId(skip: docIdSet);
    }

    final captured = <_SavedSplitWeight>[];
    for (final area in layout.layoutAreas().reversed) {
      if (area is! DockingParentArea) continue;
      final weight = area.weight;
      if (weight == null) continue;
      final leaves = _nonDocLeafIds(area, docIdSet);
      if (leaves.isEmpty) continue;
      captured.add(_SavedSplitWeight(leaves, weight));
    }

    layout.removeItemByIds(docIds);
    for (final saved in captured) {
      final target = _findAreaWithLeafIds(layout.root, saved.leafIds);
      if (target == null) continue;
      // Docking 1.16.2 stores weight on multi_split_view's Area, which only
      // exposes updateWeight, and that method is marked @internal.
      // ignore: invalid_use_of_internal_member
      target.updateWeight(saved.weight);
    }
    if (captured.isNotEmpty) layout.rebuild();
  }

  Set<dynamic> _nonDocLeafIds(DockingArea area, Set<dynamic> docIds) {
    final ids = <dynamic>{};
    void visit(DockingArea current) {
      if (current is DockingItem) {
        if (!docIds.contains(current.id)) ids.add(current.id);
        return;
      }
      if (current is DockingParentArea) {
        for (var i = 0; i < current.childrenCount; i++) {
          visit(current.childAt(i));
        }
      }
    }

    visit(area);
    return ids;
  }

  /// Deepest area whose non-doc leaves are exactly [leafIds].
  DockingArea? _findAreaWithLeafIds(DockingArea? area, Set<dynamic> leafIds) {
    if (area == null) return null;
    if (area is DockingParentArea) {
      for (var i = 0; i < area.childrenCount; i++) {
        final found = _findAreaWithLeafIds(area.childAt(i), leafIds);
        if (found != null) return found;
      }
    }
    final leaves = _nonDocLeafIds(area, const {});
    if (leaves.length != leafIds.length || !leaves.containsAll(leafIds)) {
      return null;
    }
    return area;
  }

  /// Removes every document item and leaves cores in place.
  ///
  /// Uses the same weight-preserving strip as [restore]. Project switches
  /// call this, and [DockingLayout.removeItemByIds] would otherwise drop
  /// the split weight of a group that had contained a document.
  void clearDocuments() {
    _stripDocumentsPreservingWeights();
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

  /// Selected sibling in the closed item's tab group, else any surviving item.
  ///
  /// When the closed item is the selected tab, the neighbor that would stay
  /// selected is used. A lone pane falls through to [_fallbackFocusId].
  dynamic _focusAfterClose(dynamic id) {
    final tabs = layout.findDockingTabsWithItem(id);
    if (tabs != null && tabs.childrenCount > 0) {
      final selected = tabs.selectedIndex.clamp(0, tabs.childrenCount - 1);
      if (tabs.childAt(selected).id != id) {
        return tabs.childAt(selected).id;
      }
      if (tabs.childrenCount > 1) {
        final next = selected < tabs.childrenCount - 1
            ? selected + 1
            : selected - 1;
        return tabs.childAt(next).id;
      }
    }
    return _fallbackFocusId(skip: {id});
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
      leading: dockTabLeadingForId(id),
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
    if (_disposed) return;
    // dispose is synchronous; flush the debounced write before canceling it.
    final pending = _persistTimer;
    if (pending != null) {
      unawaited(_persistQuietly());
    }
    pending?.cancel();
    _persistTimer = null;
    _disposed = true;
    layout.removeListener(_onLayoutChanged);
    super.dispose();
  }
}

/// Maps shell ids onto [DockItemWidgets]. `doc:` ids become empty stand-ins
/// so [DockingLayout.load] can finish; the controller removes them afterward.
class _ShellLayoutCodec with LayoutParserMixin, AreaBuilderMixin {
  _ShellLayoutCodec(this._widgets);

  final DockItemWidgets? _widgets;

  @override
  DockingItem buildDockingItem({
    required dynamic id,
    required double? weight,
    required bool maximized,
  }) {
    return DockingItem(
      id: id,
      name: id?.toString(),
      weight: weight,
      maximized: maximized,
      closable: true,
      keepAlive: id == DockIds.chat || DockIds.isDoc(id),
      leading: dockTabLeadingForId(id),
      widget: _widgetFor(id),
    );
  }

  Widget _widgetFor(dynamic id) {
    final widgets = _widgets;
    if (id == DockIds.threads) {
      return widgets?.threads ?? const SizedBox.shrink();
    }
    if (id == DockIds.files) {
      return widgets?.files ?? const SizedBox.shrink();
    }
    if (id == DockIds.chat) {
      return widgets?.chat ?? const SizedBox.shrink();
    }
    return const SizedBox.shrink();
  }
}

class _SavedSplitWeight {
  _SavedSplitWeight(this.leafIds, this.weight);

  final Set<dynamic> leafIds;
  final double weight;
}
