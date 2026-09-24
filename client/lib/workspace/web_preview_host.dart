import 'package:flutter/foundation.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:webview_flutter/webview_flutter.dart';

import 'preview_iframe.dart';
import 'workspace_controller.dart';

/// Preview of the catalog-served [path]. The document revision is passed
/// through so saving (or an agent-side change) reloads the served bytes
/// without recreating the platform view.
WebPreviewHost webPreviewHostFor(WorkspaceController controller, String path) {
  final uri = controller.previewUriFor(path);
  final prefix = uri.replace(query: '', fragment: '').toString();
  final cut = prefix.lastIndexOf('/preview/');
  final originPrefix = cut >= 0
      ? prefix.substring(0, cut + '/preview/'.length)
      : prefix;
  return WebPreviewHost(
    uri: uri,
    prefix: originPrefix,
    revision: controller.documentFor(path)?.revision ?? 0,
  );
}

class WebPreviewHost extends StatefulWidget {
  const WebPreviewHost({
    super.key,
    required this.uri,
    required this.prefix,
    this.revision = 0,
  });

  final Uri uri;
  final String prefix;

  /// Bumps to reload when [uri] is unchanged (the served bytes changed).
  final int revision;

  @override
  State<WebPreviewHost> createState() => _WebPreviewHostState();
}

class _WebPreviewHostState extends State<WebPreviewHost> {
  WebViewController? _controller;

  @override
  void initState() {
    super.initState();
    if (_useWebView) {
      _controller = WebViewController()
        ..setJavaScriptMode(JavaScriptMode.unrestricted)
        ..setNavigationDelegate(
          NavigationDelegate(
            onNavigationRequest: (request) {
              if (!request.url.startsWith(widget.prefix)) {
                return NavigationDecision.prevent;
              }
              return NavigationDecision.navigate;
            },
          ),
        )
        ..loadRequest(widget.uri);
    }
  }

  @override
  void didUpdateWidget(WebPreviewHost oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.uri != widget.uri) {
      _controller?.loadRequest(widget.uri);
    } else if (oldWidget.revision != widget.revision) {
      _controller?.reload();
    }
  }

  bool get _useWebView {
    if (kIsWeb) {
      return false;
    }
    switch (defaultTargetPlatform) {
      case TargetPlatform.android:
      case TargetPlatform.iOS:
      case TargetPlatform.macOS:
        return true;
      default:
        return false;
    }
  }

  @override
  Widget build(BuildContext context) {
    if (kIsWeb) {
      return PreviewIFrame(uri: widget.uri, revision: widget.revision);
    }
    final controller = _controller;
    if (controller != null) {
      return WebViewWidget(controller: controller);
    }
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text('Web preview'),
            const SizedBox(height: 8),
            SelectableText(
              widget.uri.toString(),
              key: const Key('web-preview-url'),
            ),
            const SizedBox(height: 12),
            FilledButton(
              onPressed: () => launchUrl(widget.uri),
              child: const Text('Open in browser'),
            ),
          ],
        ),
      ),
    );
  }
}
