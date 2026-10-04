import 'package:flutter_test/flutter_test.dart';

import 'package:bharatchat/core/config/env.dart';

void main() {
  test('release configuration accepts HTTPS with plaintext disabled', () {
    expect(
      () => Env.validateReleaseConfiguration(
        apiBaseUrl: 'https://api.bharatchat.example/api/v1',
        plaintextMessagingAllowed: false,
      ),
      returnsNormally,
    );
  });

  test('release configuration rejects insecure API transport', () {
    expect(
      () => Env.validateReleaseConfiguration(
        apiBaseUrl: 'http://api.bharatchat.example/api/v1',
        plaintextMessagingAllowed: false,
      ),
      throwsStateError,
    );
  });

  test('release configuration rejects plaintext messaging', () {
    expect(
      () => Env.validateReleaseConfiguration(
        apiBaseUrl: 'https://api.bharatchat.example/api/v1',
        plaintextMessagingAllowed: true,
      ),
      throwsStateError,
    );
  });
}
