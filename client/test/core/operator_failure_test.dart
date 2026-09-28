import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/core/operator_failure.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('CatalogException with validation detail is shown verbatim', () {
    final message = operatorMessageFromError(
      CatalogException(
        statusCode: 400,
        message: 'inference.repetitionPenalty must be between 1 and 2',
      ),
    );
    expect(message, 'inference.repetitionPenalty must be between 1 and 2');
  });

  test('CatalogException without useful detail keeps HTTP fallback', () {
    expect(
      operatorMessageFromError(
        CatalogException(statusCode: 500, message: 'catalog request failed'),
      ),
      'Catalog request failed (HTTP 500). Try again.',
    );
    expect(
      operatorMessageFromError(CatalogException(statusCode: 409, message: '')),
      'Catalog request failed (HTTP 409). Try again.',
    );
  });
}
