import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';

import 'package:bharatchat/core/network/api_result.dart';
import 'package:bharatchat/features/auth/domain/entities/auth_session.dart';
import 'package:bharatchat/features/auth/domain/entities/device_info.dart';
import 'package:bharatchat/features/auth/domain/repositories/auth_repository.dart';
import 'package:bharatchat/features/auth/presentation/providers/auth_providers.dart';
import 'package:bharatchat/features/auth/presentation/screens/phone_entry_screen.dart';

class _AlwaysSucceedsRepo implements AuthRepository {
  @override
  Future<ApiResult<void>> requestOtp({
    required String phoneNumber,
    required String countryCode,
  }) async => const ApiSuccess(null);

  @override
  Future<ApiResult<AuthSession>> verifyOtp({
    required String phoneNumber,
    required String countryCode,
    required String code,
    required DeviceInfoPayload device,
  }) async => ApiSuccess(
    AuthSession(
      userId: 'u1',
      isNewUser: false,
      accessToken: 'a',
      refreshToken: 'r',
      accessTokenExpiresAt: DateTime.now(),
    ),
  );

  @override
  Future<ApiResult<AuthSession>> refreshSession() async =>
      throw UnimplementedError();

  @override
  Future<ApiResult<void>> logout() async => const ApiSuccess(null);

  @override
  Future<bool> hasPersistedSession() async => false;
}

void main() {
  testWidgets('shows validation error for empty phone number', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authRepositoryProvider.overrideWithValue(_AlwaysSucceedsRepo()),
        ],
        child: const MaterialApp(home: PhoneEntryScreen()),
      ),
    );

    await tester.tap(find.text('Send OTP'));
    await tester.pump();

    expect(find.text('Phone number is required'), findsOneWidget);
  });

  testWidgets('navigates to OTP screen after successful submission', (
    tester,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authRepositoryProvider.overrideWithValue(_AlwaysSucceedsRepo()),
        ],
        child: MaterialApp.router(
          routerConfig: GoRouter(
            initialLocation: '/auth/phone-entry',
            routes: [
              GoRoute(
                path: '/auth/phone-entry',
                builder: (_, __) => const PhoneEntryScreen(),
              ),
              GoRoute(
                path: '/auth/otp-verify',
                builder: (_, __) => const Scaffold(body: Text('OTP SCREEN')),
              ),
            ],
          ),
        ),
      ),
    );

    await tester.enterText(find.byType(TextFormField).last, '9876543210');
    await tester.tap(find.text('Send OTP'));
    await tester.pumpAndSettle();

    expect(find.text('OTP SCREEN'), findsOneWidget);
  });
}
