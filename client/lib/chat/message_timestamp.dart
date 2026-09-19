import 'package:material_ui/material_ui.dart';

String formatMessageTimestamp(BuildContext context, DateTime when) {
  final loc = MaterialLocalizations.of(context);
  final local = when.toLocal();
  final date = loc.formatMediumDate(local);
  final time = loc.formatTimeOfDay(TimeOfDay.fromDateTime(local));
  return '$date, $time';
}
