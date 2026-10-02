import 'dart:convert';

import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/workspace/file_explorer.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';
import 'package:agent_fabric_client/workspace/project_files_controller.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'workspace_pane_test.dart' show MemoryWorkspaceCatalog;

Uint8List _bytes(String text) => Uint8List.fromList(utf8.encode(text));

Future<ProjectFilesController> _controller(
  MemoryWorkspaceCatalog catalog,
) async {
  final controller = ProjectFilesController(catalog: catalog);
  await controller.setProjectId('proj_1');
  return controller;
}

void main() {
  group('controller', () {
    test(
      'renaming an open dirty file keeps its buffer at the new path',
      () async {
        final catalog = MemoryWorkspaceCatalog()
          ..files['a.txt'] = _bytes('alpha');
        final controller = await _controller(catalog);
        final moved = <(OpenView, OpenView)>[];
        controller.onViewMoved = (o, n) => moved.add((o, n));
        await controller.openDefault('a.txt');
        controller.documents['a.txt']!.replaceText('alpha edited');
        final viewId = controller.focusedViewId;

        final dest = await controller.renamePath('a.txt', 'b.txt');

        expect(dest, 'b.txt');
        expect(catalog.files.keys, ['b.txt']);
        expect(controller.documents.keys, ['b.txt']);
        final doc = controller.documents['b.txt']!;
        expect(doc.path, 'b.txt');
        expect(doc.isDirty, isTrue);
        expect(doc.text, 'alpha edited');
        expect(controller.openViews.single.path, 'b.txt');
        expect(controller.focusedViewId, viewId);
        expect(controller.selectedPath, 'b.txt');
        expect(moved.single.$1.path, 'a.txt');
        expect(moved.single.$2.path, 'b.txt');
        // Saving writes to the new path, not the old one.
        await controller.savePath('b.txt');
        expect(utf8.decode(catalog.files['b.txt']!), 'alpha edited');
        expect(catalog.files.containsKey('a.txt'), isFalse);
      },
    );

    test(
      'moving a directory remaps descendants, expansion and listings',
      () async {
        final catalog = MemoryWorkspaceCatalog()
          ..dirs.addAll(['src', 'src/lib', 'dest'])
          ..files['src/lib/notes.txt'] = _bytes('notes')
          ..files['src/readme.md'] = _bytes('# hi');
        final controller = await _controller(catalog);
        await controller.expand('src');
        await controller.expand('src/lib');
        await controller.openDefault('src/lib/notes.txt');
        await controller.openDefault('src/readme.md');
        controller.documents['src/lib/notes.txt']!.replaceText('notes edited');

        await controller.movePath('src', 'dest/src');

        expect(
          controller.openViews.map((v) => v.path),
          unorderedEquals(['dest/src/lib/notes.txt', 'dest/src/readme.md']),
        );
        expect(
          controller.documents['dest/src/lib/notes.txt']!.text,
          'notes edited',
        );
        expect(controller.expanded, containsAll(['dest/src', 'dest/src/lib']));
        expect(controller.expanded.contains('src'), isFalse);
        expect(controller.children.containsKey('src'), isFalse);
        expect(controller.selectedPath, startsWith('dest/src'));
      },
    );

    test('a collision throws and leaves local state untouched', () async {
      final catalog = MemoryWorkspaceCatalog()
        ..files['a.txt'] = _bytes('alpha')
        ..files['b.txt'] = _bytes('bravo');
      final controller = await _controller(catalog);
      await controller.openDefault('a.txt');
      controller.documents['a.txt']!.replaceText('unsaved');

      await expectLater(
        controller.renamePath('a.txt', 'b.txt'),
        throwsA(
          isA<CatalogException>().having((e) => e.statusCode, 'status', 409),
        ),
      );

      expect(controller.documents['a.txt']!.text, 'unsaved');
      expect(controller.documents['a.txt']!.isDirty, isTrue);
      expect(controller.openViews.single.path, 'a.txt');
      expect(utf8.decode(catalog.files['b.txt']!), 'bravo');
    });

    test(
      'invalid names and moves into itself are rejected before the call',
      () async {
        final catalog = MemoryWorkspaceCatalog()
          ..dirs.add('src')
          ..files['a.txt'] = _bytes('x');
        final controller = await _controller(catalog);
        await expectLater(
          controller.renamePath('a.txt', 'x/y'),
          throwsA(isA<CatalogException>()),
        );
        await expectLater(
          controller.movePath('src', 'src/inner'),
          throwsA(isA<CatalogException>()),
        );
        expect(catalog.files.keys, ['a.txt']);
      },
    );

    test('createFile refuses to overwrite a listed file', () async {
      final catalog = MemoryWorkspaceCatalog()..files['a.txt'] = _bytes('keep');
      final controller = await _controller(catalog);
      await expectLater(
        controller.createFile('.', 'a.txt'),
        throwsA(isA<CatalogException>()),
      );
      expect(utf8.decode(catalog.files['a.txt']!), 'keep');
    });

    test('deleting a directory closes views of its descendants', () async {
      final catalog = MemoryWorkspaceCatalog()
        ..dirs.add('src')
        ..files['src/a.txt'] = _bytes('x');
      final controller = await _controller(catalog);
      await controller.expand('src');
      await controller.openDefault('src/a.txt');
      catalog.files.remove('src/a.txt');

      await controller.deletePath('src');

      expect(controller.openViews, isEmpty);
      expect(controller.documents, isEmpty);
      expect(controller.expanded.contains('src'), isFalse);
    });

    test('duplicatePath selects and opens the copy', () async {
      final catalog = MemoryWorkspaceCatalog()
        ..files['index.html'] = _bytes('<h1>');
      final controller = await _controller(catalog);

      final first = await controller.duplicatePath('index.html');
      final second = await controller.duplicatePath('index.html');

      expect(first, 'index copy.html');
      expect(second, 'index copy 2.html');
      expect(controller.selectedPath, 'index copy 2.html');
      expect(controller.openViews.last.path, 'index copy 2.html');
    });
  });

  group('explorer', () {
    Future<(MemoryWorkspaceCatalog, ProjectFilesController)> pump(
      WidgetTester tester,
      void Function(MemoryWorkspaceCatalog catalog) seed,
    ) async {
      tester.view.physicalSize = const Size(1200, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final catalog = MemoryWorkspaceCatalog();
      seed(catalog);
      final controller = await _controller(catalog);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(body: FileExplorer(controller: controller)),
        ),
      );
      await tester.pumpAndSettle();
      return (catalog, controller);
    }

    testWidgets('F2 renames inline and the open document follows', (
      tester,
    ) async {
      final (catalog, controller) = await pump(
        tester,
        (c) => c.files['a.txt'] = _bytes('alpha'),
      );
      await tester.tap(find.byKey(const Key('file-row-a.txt')));
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.f2);
      await tester.pumpAndSettle();

      final field = find.byKey(const Key('create-name-field'));
      expect(field, findsOneWidget);
      // Only the stem is pre-selected so the extension survives typing.
      final text = tester.widget<TextField>(field).controller!;
      expect(text.selection.start, 0);
      expect(text.selection.end, 1);

      await tester.enterText(field, 'b.txt');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pumpAndSettle();

      expect(catalog.files.keys, ['b.txt']);
      expect(controller.openViews.single.path, 'b.txt');
      expect(find.byKey(const Key('file-row-b.txt')), findsOneWidget);
      expect(find.byKey(const Key('create-name-field')), findsNothing);
    });

    testWidgets(
      'a rename collision stays open with the error and keeps edits',
      (tester) async {
        final (catalog, controller) = await pump(tester, (c) {
          c.files['a.txt'] = _bytes('alpha');
          c.files['b.txt'] = _bytes('bravo');
        });
        await tester.tap(find.byKey(const Key('file-row-a.txt')));
        await tester.pumpAndSettle();
        controller.documents['a.txt']!.replaceText('unsaved');
        await tester.sendKeyEvent(LogicalKeyboardKey.f2);
        await tester.pumpAndSettle();
        await tester.enterText(
          find.byKey(const Key('create-name-field')),
          'b.txt',
        );
        await tester.testTextInput.receiveAction(TextInputAction.done);
        await tester.pumpAndSettle();

        expect(find.byKey(const Key('inline-edit-error')), findsOneWidget);
        expect(find.byKey(const Key('create-name-field')), findsOneWidget);
        expect(catalog.files.keys, unorderedEquals(['a.txt', 'b.txt']));
        expect(controller.documents['a.txt']!.text, 'unsaved');
      },
    );

    testWidgets('Escape cancels an inline rename', (tester) async {
      final (catalog, _) = await pump(
        tester,
        (c) => c.files['a.txt'] = _bytes('alpha'),
      );
      await tester.tap(find.byKey(const Key('file-row-a.txt')));
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.f2);
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.escape);
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('create-name-field')), findsNothing);
      expect(catalog.files.keys, ['a.txt']);
    });

    testWidgets('new folder is created inline under the selected folder', (
      tester,
    ) async {
      final (catalog, controller) = await pump(tester, (c) {
        c.dirs.add('src');
      });
      await tester.tap(find.byKey(const Key('file-row-src')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('explorer-new-folder')));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('create-name-field')), 'lib');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pumpAndSettle();

      expect(catalog.dirs, contains('src/lib'));
      expect(controller.selectedPath, 'src/lib');
    });

    testWidgets('arrow keys move selection, Enter opens, Delete confirms', (
      tester,
    ) async {
      final (catalog, controller) = await pump(tester, (c) {
        c.dirs.add('src');
        c.files['a.txt'] = _bytes('alpha');
      });
      // Focus the tree without picking a row.
      await tester.tap(find.byKey(const Key('file-row-src')));
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await tester.pumpAndSettle();
      expect(controller.selectedPath, 'a.txt');

      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pumpAndSettle();
      expect(controller.openViews.single.path, 'a.txt');

      await tester.sendKeyEvent(LogicalKeyboardKey.arrowUp);
      await tester.pumpAndSettle();
      expect(controller.selectedPath, 'src');

      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await tester.sendKeyEvent(LogicalKeyboardKey.delete);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('delete-confirm')));
      await tester.pumpAndSettle();
      expect(catalog.files, isEmpty);
    });

    testWidgets('context menu duplicates and renames', (tester) async {
      final (catalog, _) = await pump(
        tester,
        (c) => c.files['index.html'] = _bytes('<h1>'),
      );
      await tester.tap(find.byKey(const Key('file-menu-index.html')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('menu-duplicate')));
      await tester.pumpAndSettle();
      expect(catalog.files.keys, contains('index copy.html'));

      await tester.tap(find.byKey(const Key('file-menu-index.html')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('menu-rename')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('create-name-field')), findsOneWidget);
    });

    testWidgets('Move... picks a folder and moves the entry', (tester) async {
      final (catalog, controller) = await pump(tester, (c) {
        c.dirs.addAll(['docs', 'docs/guides']);
        c.files['a.txt'] = _bytes('alpha');
      });
      await tester.tap(find.byKey(const Key('file-menu-a.txt')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('menu-move')));
      await tester.pumpAndSettle();
      // Moving to where it already is stays disabled.
      expect(
        tester
            .widget<FilledButton>(
              find.byKey(const Key('folder-picker-confirm')),
            )
            .onPressed,
        isNull,
      );
      await tester.tap(find.byKey(const Key('folder-picker-docs')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('folder-picker-guides')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('folder-picker-confirm')));
      await tester.pumpAndSettle();

      expect(catalog.files.keys, ['docs/guides/a.txt']);
      expect(controller.expanded, contains('docs/guides'));
    });

    testWidgets('dragging a file onto a folder moves it', (tester) async {
      final (catalog, controller) = await pump(tester, (c) {
        c.dirs.add('src');
        c.files['a.txt'] = _bytes('alpha');
      });
      final from = tester.getCenter(find.byKey(const Key('file-row-a.txt')));
      final to = tester.getCenter(find.byKey(const Key('file-row-src')));
      final gesture = await tester.startGesture(from);
      // Tests run as Android, where a move starts on long press.
      await tester.pump(kLongPressTimeout + const Duration(milliseconds: 50));
      await gesture.moveTo(to - const Offset(0, 4));
      await tester.pump();
      await gesture.moveTo(to);
      await tester.pump();
      await gesture.up();
      await tester.pumpAndSettle();

      expect(catalog.files.keys, ['src/a.txt']);
      expect(controller.expanded, contains('src'));
    });

    testWidgets('dragging onto the project header moves to the root', (
      tester,
    ) async {
      final (catalog, _) = await pump(tester, (c) {
        c.dirs.add('src');
        c.files['src/a.txt'] = _bytes('alpha');
      });
      await tester.tap(find.byKey(const Key('file-row-src')));
      await tester.pumpAndSettle();
      final from = tester.getCenter(
        find.byKey(const Key('file-row-src/a.txt')),
      );
      final to = tester.getCenter(find.byKey(const Key('explorer-root')));
      final gesture = await tester.startGesture(from);
      await tester.pump(kLongPressTimeout + const Duration(milliseconds: 50));
      await gesture.moveTo(to + const Offset(0, 4));
      await tester.pump();
      await gesture.moveTo(to);
      await tester.pump();
      await gesture.up();
      await tester.pumpAndSettle();

      expect(catalog.files.keys, ['a.txt']);
    });

    testWidgets('dragging a folder onto itself is ignored', (tester) async {
      final (catalog, _) = await pump(tester, (c) {
        c.dirs.addAll(['src', 'src/lib']);
      });
      final from = tester.getCenter(find.byKey(const Key('file-row-src')));
      final gesture = await tester.startGesture(from);
      // Tests run as Android, where a move starts on long press.
      await tester.pump(kLongPressTimeout + const Duration(milliseconds: 50));
      await gesture.moveTo(from + const Offset(0, 6));
      await tester.pump();
      await gesture.up();
      await tester.pumpAndSettle();
      expect(catalog.dirs, containsAll(['src', 'src/lib']));
    });
  });
}
