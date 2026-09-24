import 'package:material_ui/material_ui.dart';

class PreviewIFrame extends StatelessWidget {
  const PreviewIFrame({super.key, required this.uri, this.revision = 0});

  final Uri uri;
  final int revision;

  @override
  Widget build(BuildContext context) {
    return const SizedBox.shrink();
  }
}
