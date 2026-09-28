import 'package:agent_fabric_client/settings/inference_param_row.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('slider commits value into the text field', (tester) async {
    final controller = TextEditingController();
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: InferenceParamRow(
            fieldKey: const Key('param'),
            label: 'Temperature',
            tooltip: 'Controls randomness.',
            controller: controller,
            min: 0,
            max: 2,
            unsetDisplay: 1,
            divisions: 200,
            onChanged: () {},
          ),
        ),
      ),
    );

    expect(find.byTooltip('Controls randomness.'), findsOneWidget);
    expect(controller.text, isEmpty);

    await tester.drag(find.byType(Slider), const Offset(80, 0));
    await tester.pumpAndSettle();
    expect(controller.text, isNotEmpty);

    await tester.enterText(find.byKey(const Key('param')), '0.42');
    await tester.pump();
    expect(controller.text, '0.42');
  });
}
