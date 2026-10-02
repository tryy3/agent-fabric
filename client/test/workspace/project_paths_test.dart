import 'package:agent_fabric_client/workspace/project_paths.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('join, parent and base name', () {
    expect(joinProjectPath('.', 'a.txt'), 'a.txt');
    expect(joinProjectPath('', 'a.txt'), 'a.txt');
    expect(joinProjectPath('src', 'a.txt'), 'src/a.txt');
    expect(parentOfPath('a.txt'), '.');
    expect(parentOfPath('src/a.txt'), 'src');
    expect(parentOfPath('src/lib/a.txt'), 'src/lib');
    expect(baseNameOfPath('src/lib/a.txt'), 'a.txt');
    expect(baseNameOfPath('a.txt'), 'a.txt');
  });

  test('remapPath rewrites only the moved subtree', () {
    expect(remapPath('src', 'src', 'app'), 'app');
    expect(remapPath('src/a.txt', 'src', 'app'), 'app/a.txt');
    expect(remapPath('src/x/a.txt', 'src', 'lib/src'), 'lib/src/x/a.txt');
    expect(remapPath('srcs/a.txt', 'src', 'app'), 'srcs/a.txt');
    expect(remapPath('other.txt', 'src', 'app'), 'other.txt');
  });

  test('isSameOrDescendant respects segment boundaries', () {
    expect(isSameOrDescendant('src', 'src'), isTrue);
    expect(isSameOrDescendant('src/a', 'src'), isTrue);
    expect(isSameOrDescendant('srcs', 'src'), isFalse);
  });

  test('renameSelectionLength keeps the extension out of the selection', () {
    expect(renameSelectionLength('index.html'), 'index'.length);
    expect(renameSelectionLength('a.tar.gz'), 'a.tar'.length);
    expect(renameSelectionLength('.env'), '.env'.length);
    expect(renameSelectionLength('Makefile'), 'Makefile'.length);
  });

  test('validateEntryName', () {
    expect(validateEntryName('ok.txt'), isNull);
    expect(validateEntryName('  '), isNotNull);
    expect(validateEntryName('..'), isNotNull);
    expect(validateEntryName('a/b'), isNotNull);
    expect(validateEntryName(r'a\b'), isNotNull);
  });
}
