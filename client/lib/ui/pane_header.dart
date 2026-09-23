import 'package:material_ui/material_ui.dart';

/// Leading title plus trailing actions that stay inside a narrow dock column.
///
/// The title keeps enough width to ellipsize. Actions scroll horizontally
/// instead of forcing the row wider than the pane.
class PaneHeader extends StatelessWidget {
  const PaneHeader({super.key, required this.title, required this.actions});

  final Widget title;
  final List<Widget> actions;

  static const double actionExtent = 32;

  static final ButtonStyle actionStyle = IconButton.styleFrom(
    visualDensity: VisualDensity.compact,
    padding: EdgeInsets.zero,
    minimumSize: const Size(actionExtent, actionExtent),
    fixedSize: const Size(actionExtent, actionExtent),
    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
  );

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth.isFinite
            ? constraints.maxWidth
            : 0.0;
        // DropdownButton's arrow is 24px; narrower than that overflows its row.
        var titleWidth = width * 0.42;
        if (width >= 28 && titleWidth < 28) {
          titleWidth = 28;
        }
        if (titleWidth > width) {
          titleWidth = width;
        }
        return SizedBox(
          height: actionExtent,
          child: Row(
            children: [
              SizedBox(width: titleWidth, child: title),
              SizedBox(
                width: width - titleWidth,
                child: SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  reverse: true,
                  child: Row(mainAxisSize: MainAxisSize.min, children: actions),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
