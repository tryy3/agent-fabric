import 'package:material_ui/material_ui.dart';
import 'package:pointer_interceptor/pointer_interceptor.dart';
import 'package:web/web.dart' as web;

class PreviewIFrame extends StatelessWidget {
  const PreviewIFrame({super.key, required this.uri});

  final Uri uri;

  @override
  Widget build(BuildContext context) {
    return PointerInterceptor(
      child: HtmlElementView.fromTagName(
        tagName: 'iframe',
        onElementCreated: (element) {
          final iframe = element as web.HTMLIFrameElement;
          iframe.src = uri.toString();
          iframe.setAttribute('sandbox', 'allow-scripts');
          iframe.style.border = '0';
          iframe.style.width = '100%';
          iframe.style.height = '100%';
        },
      ),
    );
  }
}
