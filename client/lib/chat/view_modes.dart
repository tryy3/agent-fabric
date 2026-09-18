import 'display_settings.dart';

enum ToolIOMode { input, output, both }

class ViewMode {
  const ViewMode({
    required this.id,
    required this.label,
    required this.description,
    required this.markdownRender,
    required this.thinkingVisibility,
    required this.toolVisibility,
    required this.toolIO,
    this.rawRequests = false,
  });

  final String id;
  final String label;
  final String description;
  final bool markdownRender;
  final VisibilityMode thinkingVisibility;
  final VisibilityMode toolVisibility;
  final ToolIOMode toolIO;
  final bool rawRequests;
}

const kDefaultViewModeId = 'pretty';

const kBuiltInViewModes = <ViewMode>[
  ViewMode(
    id: 'pretty',
    label: 'Pretty',
    description: 'Rendered markdown, quiet harness',
    markdownRender: true,
    thinkingVisibility: VisibilityMode.collapsed,
    toolVisibility: VisibilityMode.collapsed,
    toolIO: ToolIOMode.both,
  ),
  ViewMode(
    id: 'detailed',
    label: 'Detailed',
    description: 'Plain text, more inspectable',
    markdownRender: false,
    thinkingVisibility: VisibilityMode.collapsed,
    toolVisibility: VisibilityMode.collapsed,
    toolIO: ToolIOMode.both,
  ),
];

ViewMode resolveViewMode(
  String? threadViewModeId, {
  String appDefaultId = kDefaultViewModeId,
}) {
  final id = threadViewModeId ?? appDefaultId;
  for (final mode in kBuiltInViewModes) {
    if (mode.id == id) {
      return mode;
    }
  }
  for (final mode in kBuiltInViewModes) {
    if (mode.id == appDefaultId) {
      return mode;
    }
  }
  return kBuiltInViewModes.first;
}
