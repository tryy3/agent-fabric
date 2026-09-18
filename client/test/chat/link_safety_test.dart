import 'package:agent_fabric_client/chat/link_safety.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('inspectMarkdownLink', () {
    test('parses https and has no warnings', () {
      final link = inspectMarkdownLink(
        href: 'https://example.com/path',
        linkText: 'docs',
      );
      expect(link.uri?.host, 'example.com');
      expect(link.displayUrl, 'https://example.com/path');
      expect(link.warnings, isEmpty);
      expect(link.canLaunch, isTrue);
    });

    test('warns on non-http scheme without blocking', () {
      final link = inspectMarkdownLink(href: 'javascript:alert(1)');
      expect(link.uri?.scheme, 'javascript');
      expect(link.canLaunch, isTrue);
      expect(
        link.warnings.any((w) => w.contains('javascript')),
        isTrue,
      );
    });

    test('warns when link text host disagrees with href', () {
      final link = inspectMarkdownLink(
        href: 'https://evil.example/phish',
        linkText: 'https://openai.com',
      );
      expect(
        link.warnings.any((w) => w.contains('openai.com')),
        isTrue,
      );
      expect(
        link.warnings.any((w) => w.contains('evil.example')),
        isTrue,
      );
    });

    test('warns on credentials in URL', () {
      final link = inspectMarkdownLink(
        href: 'https://user:pass@example.com/',
      );
      expect(link.warnings.any((w) => w.contains('credentials')), isTrue);
    });

    test('warns on empty href', () {
      final link = inspectMarkdownLink(href: '  ');
      expect(link.canLaunch, isFalse);
      expect(link.warnings, isNotEmpty);
    });

    test('promotes www. host to https', () {
      final link = inspectMarkdownLink(href: 'www.example.com/a');
      expect(link.uri?.scheme, 'https');
      expect(link.uri?.host, 'www.example.com');
    });
  });
}
