import 'package:material_ui/material_ui.dart';
import 'package:pointer_interceptor/pointer_interceptor.dart';
import 'package:web/web.dart' as web;

class PreviewIFrame extends StatefulWidget {
  const PreviewIFrame({super.key, required this.uri, this.revision = 0});

  final Uri uri;

  /// Bumps to reload when [uri] is unchanged (the served bytes changed).
  final int revision;

  @override
  State<PreviewIFrame> createState() => _PreviewIFrameState();
}

class _PreviewIFrameState extends State<PreviewIFrame> {
  web.HTMLIFrameElement? _iframe;

  @override
  void didUpdateWidget(PreviewIFrame oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.uri != widget.uri || oldWidget.revision != widget.revision) {
      // Reassigning src reloads the document in place. Recreating the
      // platform view instead churns CanvasKit overlay surfaces, which
      // spams "Shader compilation error" on web.
      _iframe?.src = widget.uri.toString();
    }
  }

  @override
  Widget build(BuildContext context) {
    return PointerInterceptor(
      child: HtmlElementView.fromTagName(
        tagName: 'iframe',
        onElementCreated: (element) {
          final iframe = element as web.HTMLIFrameElement;
          iframe.src = widget.uri.toString();
          iframe.setAttribute('sandbox', 'allow-scripts');
          iframe.style.border = '0';
          iframe.style.width = '100%';
          iframe.style.height = '100%';
          // Iframes are transparent on the web; default to a browser-like
          // white so pages without their own background stay readable.
          iframe.style.backgroundColor = 'white';
          _iframe = iframe;
        },
      ),
    );
  }
}
