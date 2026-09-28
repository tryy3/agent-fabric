import '../catalog/models.dart';

/// Stable failure codes for catalog / workspace surfaces (not localized strings).
sealed class OperatorFailure {
  const OperatorFailure();

  String get code;
}

final class CatalogRequestFailure extends OperatorFailure {
  const CatalogRequestFailure({this.statusCode, this.message = ''});

  final int? statusCode;

  /// Server-provided operator-facing detail when present (e.g. validation).
  final String message;

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
    return CatalogRequestFailure(
      statusCode: error.statusCode,
      message: error.message.trim(),
    );
  }
  return const UnknownOperatorFailure();
}

bool _isGenericCatalogMessage(String message) {
  if (message.isEmpty) {
    return true;
  }
  final lower = message.toLowerCase();
  return lower == 'catalog request failed' ||
      lower == 'request failed' ||
      lower.startsWith('catalogexception(');
}

/// Operator-facing copy for a failure code. Keep free of paths/ids/stacks.
String operatorMessageFor(OperatorFailure failure) {
  return switch (failure) {
    CatalogRequestFailure(:final statusCode, :final message) =>
      !_isGenericCatalogMessage(message)
          ? message
          : statusCode == null
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
