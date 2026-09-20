import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import 'sandbox_overlay_form.dart';

export 'sandbox_overlay_form.dart' show newSandboxVolumeID, newSandboxPathID;

class SandboxTab extends StatefulWidget {
  const SandboxTab({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  State<SandboxTab> createState() => _SandboxTabState();
}

class _SandboxTabState extends State<SandboxTab> {
  Map<String, dynamic>? _sandbox;
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  Future<void> _reload() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final settings = await widget.catalog.getSettings();
      if (!mounted) {
        return;
      }
      setState(() {
        _sandbox = Map<String, dynamic>.from(settings.sandbox);
        _loading = false;
      });
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (_sandbox == null) {
      return Scaffold(
        body: Center(child: Text(_error ?? 'Failed to load sandbox settings')),
      );
    }
    return Scaffold(
      body: SandboxOverlayForm(
        initial: _sandbox!,
        heading: 'Global sandbox defaults',
        subtitle: 'These apply to the next prompt. Project and agent overlays can still override them.',
        onSave: (sandbox) async {
          final updated = await widget.catalog.patchSettings(sandbox: sandbox);
          if (!mounted) {
            return;
          }
          setState(() {
            _sandbox = Map<String, dynamic>.from(updated.sandbox);
          });
        },
      ),
    );
  }
}
