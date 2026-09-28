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

  testWidgets('clear control slot does not shift the number field', (
    tester,
  ) async {
    final controller = TextEditingController();
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) {
              return InferenceParamRow(
                fieldKey: const Key('param'),
                label: 'Temperature',
                tooltip: 'Controls randomness.',
                controller: controller,
                min: 0,
                max: 2,
                unsetDisplay: 1,
                divisions: 200,
                onChanged: () => setState(() {}),
              );
            },
          ),
        ),
      ),
    );

    final unsetDx = tester.getTopLeft(find.byKey(const Key('param'))).dx;
    await tester.enterText(find.byKey(const Key('param')), '1.2');
    await tester.pump();
    final setDx = tester.getTopLeft(find.byKey(const Key('param'))).dx;
    expect(setDx, unsetDx);
    expect(find.byTooltip('Use provider default'), findsOneWidget);
  });

  testWidgets('typed value above slider max is preserved in the field', (
    tester,
  ) async {
    final controller = TextEditingController();
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) {
              return InferenceParamRow(
                fieldKey: const Key('param'),
                label: 'Top K',
                tooltip: 'Top K sampling.',
                controller: controller,
                min: 0,
                max: 100,
                unsetDisplay: 20,
                integer: true,
                divisions: 100,
                onChanged: () => setState(() {}),
              );
            },
          ),
        ),
      ),
    );

    await tester.enterText(find.byKey(const Key('param')), '250');
    await tester.pump();
    expect(controller.text, '250');

    final slider = tester.widget<Slider>(find.byType(Slider));
    expect(slider.value, 100);
    expect(slider.label, '250');
  });
}
