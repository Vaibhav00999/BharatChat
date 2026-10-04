import 'package:flutter/foundation.dart';

class Env {
  static const String baseUrl = String.fromEnvironment(
    'BASE_URL',
    defaultValue: 'http://10.0.2.2:8080/api/v1',
  );

  /// Plaintext messaging exists only to support local UI development while the
  /// audited native E2EE bridge is unfinished. Release builds reject this flag.
  static const bool allowPlaintextMessaging = bool.fromEnvironment(
    'ALLOW_PLAINTEXT_MESSAGING',
  );

  static void validate() {
    if (!kReleaseMode) return;

    validateReleaseConfiguration(
      apiBaseUrl: baseUrl,
      plaintextMessagingAllowed: allowPlaintextMessaging,
    );
  }

  static void validateReleaseConfiguration({
    required String apiBaseUrl,
    required bool plaintextMessagingAllowed,
  }) {
    final uri = Uri.tryParse(apiBaseUrl);
    if (uri == null || uri.scheme != 'https' || uri.host.isEmpty) {
      throw StateError('Release builds require an absolute HTTPS BASE_URL');
    }
    if (plaintextMessagingAllowed) {
      throw StateError(
        'ALLOW_PLAINTEXT_MESSAGING must never be enabled in release builds',
      );
    }
  }
}
