import '../catalog/models.dart';

/// Stable failure codes for catalog / workspace surfaces (not localized strings).
sealed class OperatorFailure {
  const OperatorFailure();

  String get code;
}

final class CatalogRequestFailure extends OperatorFailure {
  const CatalogRequestFailure({this.statusCode});

  final int? statusCode;

  @override
  String get code => 'catalog.request_failed';
}

final class WorkspaceIoFailure extends OperatorFailure {
  const WorkspaceIoFailure();

  @override
  String get code => 'workspace.io_failed';
}

final class UnknownOperatorFailure extends OperatorFailure {
  const UnknownOperatorFailure();

  @override
  String get code => 'operator.unknown';
}

/// Maps a thrown object to a typed failure (log the original separately).
OperatorFailure operatorFailureFrom(Object error) {
  if (error is CatalogException) {
    return CatalogRequestFailure(statusCode: error.statusCode);
  }
  return const UnknownOperatorFailure();
}

/// Operator-facing copy for a failure code. Keep free of paths/ids/stacks.
String operatorMessageFor(OperatorFailure failure) {
  return switch (failure) {
    CatalogRequestFailure(:final statusCode) =>
      statusCode == null
          ? 'Could not reach the catalog. Check the control plane and try again.'
          : 'Catalog request failed (HTTP $statusCode). Try again.',
    WorkspaceIoFailure() =>
      'Could not read or write workspace files. Try again.',
    UnknownOperatorFailure() =>
      'Something went wrong. Check the connection and try again.',
  };
}

/// Convenience: map a thrown object straight to operator copy.
String operatorMessageFromError(Object error) {
  return operatorMessageFor(operatorFailureFrom(error));
}
