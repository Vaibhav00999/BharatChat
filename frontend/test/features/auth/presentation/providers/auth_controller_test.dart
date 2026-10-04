import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:bharatchat/core/network/api_result.dart';
import 'package:bharatchat/features/auth/domain/entities/auth_session.dart';
import 'package:bharatchat/features/auth/domain/entities/device_info.dart';
import 'package:bharatchat/features/auth/domain/repositories/auth_repository.dart';
import 'package:bharatchat/features/auth/presentation/providers/auth_providers.dart';

class FakeAuthRepository implements AuthRepository {
  bool otpRequestShouldFail = false;
  bool verifyShouldFail = false;
  bool nextIsNewUser = false;
  bool persistedSessionExists = false;

  @override
  Future<ApiResult<void>> requestOtp({
    required String phoneNumber,
    required String countryCode,
  }) async {
    if (otpRequestShouldFail) {
      return const ApiFailure(
        code: 'TOO_MANY_REQUESTS',
        message: 'Please wait before requesting another OTP',
      );
    }
    return const ApiSuccess(null);
  }

  @override
  Future<ApiResult<AuthSession>> verifyOtp({
    required String phoneNumber,
    required String countryCode,
    required String code,
    required DeviceInfoPayload device,
  }) async {
    if (verifyShouldFail) {
      return const ApiFailure(
        code: 'VALIDATION_ERROR',
        message: 'Incorrect OTP code',
      );
    }
    return ApiSuccess(
      AuthSession(
        userId: 'user-123',
        isNewUser: nextIsNewUser,
        accessToken: 'fake-access-token',
        refreshToken: 'fake-refresh-token',
        accessTokenExpiresAt: DateTime.now().add(const Duration(minutes: 15)),
      ),
    );
  }

  @override
  Future<ApiResult<AuthSession>> refreshSession() async {
    return ApiSuccess(
      AuthSession(
        userId: 'user-123',
        isNewUser: false,
        accessToken: 'refreshed-access-token',
        refreshToken: 'refreshed-refresh-token',
        accessTokenExpiresAt: DateTime.now().add(const Duration(minutes: 15)),
      ),
    );
  }

  @override
  Future<ApiResult<void>> logout() async => const ApiSuccess(null);

  @override
  Future<bool> hasPersistedSession() async => persistedSessionExists;
}

void main() {
  late FakeAuthRepository fakeRepo;
  late ProviderContainer container;

  setUp(() {
    fakeRepo = FakeAuthRepository();
    container = ProviderContainer(
      overrides: [authRepositoryProvider.overrideWithValue(fakeRepo)],
    );
  });

  tearDown(() => container.dispose());

  test(
    'bootstraps to unauthenticated when no persisted session exists',
    () async {
      fakeRepo.persistedSessionExists = false;
      final controller = container.read(authControllerProvider.notifier);
      await Future<void>.delayed(Duration.zero);

      expect(
        container.read(authControllerProvider).status,
        AuthStatus.unauthenticated,
      );
      expect(controller, isNotNull);
    },
  );

  test('bootstraps to authenticated when a persisted session exists', () async {
    fakeRepo.persistedSessionExists = true;
    final controller = container.read(authControllerProvider.notifier);
    await Future<void>.delayed(Duration.zero);

    expect(
      container.read(authControllerProvider).status,
      AuthStatus.authenticated,
    );
    expect(controller, isNotNull);
  });

  test('requestOtp transitions to otpSent on success', () async {
    final controller = container.read(authControllerProvider.notifier);
    await controller.requestOtp('+919876543210', '+91');

    final state = container.read(authControllerProvider);
    expect(state.status, AuthStatus.otpSent);
    expect(state.phoneNumber, '+919876543210');
    expect(state.errorMessage, isNull);
  });

  test('requestOtp surfaces error message on failure', () async {
    fakeRepo.otpRequestShouldFail = true;
    final controller = container.read(authControllerProvider.notifier);
    await controller.requestOtp('+919876543210', '+91');

    final state = container.read(authControllerProvider);
    expect(state.status, isNot(AuthStatus.otpSent));
    expect(state.errorMessage, 'Please wait before requesting another OTP');
  });

  test('verifyOtp transitions to needsProfileSetup for new users', () async {
    fakeRepo.nextIsNewUser = true;
    final controller = container.read(authControllerProvider.notifier);
    await controller.requestOtp('+919876543210', '+91');
    await controller.verifyOtp(
      '123456',
      const DeviceInfoPayload(
        platform: 'android',
        deviceName: 'test',
        appVersion: '1.0.0',
      ),
    );

    expect(
      container.read(authControllerProvider).status,
      AuthStatus.needsProfileSetup,
    );
    expect(container.read(authControllerProvider).userId, 'user-123');
  });

  test(
    'verifyOtp transitions directly to authenticated for existing users',
    () async {
      fakeRepo.nextIsNewUser = false;
      final controller = container.read(authControllerProvider.notifier);
      await controller.requestOtp('+919876543210', '+91');
      await controller.verifyOtp(
        '123456',
        const DeviceInfoPayload(
          platform: 'android',
          deviceName: 'test',
          appVersion: '1.0.0',
        ),
      );

      expect(
        container.read(authControllerProvider).status,
        AuthStatus.authenticated,
      );
    },
  );

  test('verifyOtp surfaces error on wrong code', () async {
    fakeRepo.verifyShouldFail = true;
    final controller = container.read(authControllerProvider.notifier);
    await controller.requestOtp('+919876543210', '+91');
    await controller.verifyOtp(
      '000000',
      const DeviceInfoPayload(
        platform: 'android',
        deviceName: 'test',
        appVersion: '1.0.0',
      ),
    );

    expect(
      container.read(authControllerProvider).errorMessage,
      'Incorrect OTP code',
    );
  });

  test('logout resets state to unauthenticated', () async {
    final controller = container.read(authControllerProvider.notifier);
    await controller.requestOtp('+919876543210', '+91');
    await controller.verifyOtp(
      '123456',
      const DeviceInfoPayload(
        platform: 'android',
        deviceName: 'test',
        appVersion: '1.0.0',
      ),
    );

    await controller.logout();
    expect(
      container.read(authControllerProvider).status,
      AuthStatus.unauthenticated,
    );
  });

  test('logout erases account-scoped local session data', () async {
    var cleared = false;
    final controller = AuthController(
      fakeRepo,
      clearLocalSessionData: () async => cleared = true,
    );
    await Future<void>.delayed(Duration.zero);

    await controller.logout();

    expect(cleared, isTrue);
    expect(controller.state.status, AuthStatus.unauthenticated);
    controller.dispose();
  });
}
