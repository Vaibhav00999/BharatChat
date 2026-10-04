import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/network/api_result.dart';
import '../../../../core/network/dio_client.dart';
import '../../../../core/storage/hive_boxes.dart';
import '../../../../core/storage/secure_storage.dart';
import '../../data/datasources/auth_remote_datasource.dart';
import '../../data/repositories/auth_repository_impl.dart';
import '../../domain/entities/device_info.dart';
import '../../domain/repositories/auth_repository.dart';

final Provider<SecureTokenStorage> secureTokenStorageProvider =
    Provider<SecureTokenStorage>((ref) => SecureTokenStorage());
final Provider<Dio> dioProvider = Provider<Dio>(
  (ref) => DioClientFactory.create(
    tokenStorage: ref.read(secureTokenStorageProvider),
    onUnauthorized: () async {
      final result = await ref.read(authRepositoryProvider).refreshSession();
      return result is ApiSuccess;
    },
  ),
);
final Provider<AuthRemoteDataSource> authRemoteDataSourceProvider =
    Provider<AuthRemoteDataSource>(
      (ref) => AuthRemoteDataSource(ref.read(dioProvider)),
    );
final Provider<AuthRepository> authRepositoryProvider =
    Provider<AuthRepository>(
      (ref) => AuthRepositoryImpl(
        ref.read(authRemoteDataSourceProvider),
        ref.read(secureTokenStorageProvider),
      ),
    );

enum AuthStatus {
  unknown,
  unauthenticated,
  otpSent,
  authenticated,
  needsProfileSetup,
}

class AuthState {
  final AuthStatus status;
  final String? phoneNumber, countryCode, userId, errorMessage;
  final bool isLoading;
  const AuthState({
    this.status = AuthStatus.unknown,
    this.phoneNumber,
    this.countryCode,
    this.userId,
    this.errorMessage,
    this.isLoading = false,
  });
  AuthState copyWith({
    AuthStatus? status,
    String? phoneNumber,
    String? countryCode,
    String? userId,
    String? errorMessage,
    bool? isLoading,
  }) => AuthState(
    status: status ?? this.status,
    phoneNumber: phoneNumber ?? this.phoneNumber,
    countryCode: countryCode ?? this.countryCode,
    userId: userId ?? this.userId,
    errorMessage: errorMessage,
    isLoading: isLoading ?? false,
  );
}

class AuthController extends StateNotifier<AuthState> {
  final AuthRepository _repo;
  final Future<void> Function() _clearLocalSessionData;
  AuthController(this._repo, {Future<void> Function()? clearLocalSessionData})
    : _clearLocalSessionData =
          clearLocalSessionData ?? HiveBoxes.clearSessionData,
      super(const AuthState()) {
    _bootstrap();
  }
  Future<void> _bootstrap() async {
    if (!await _repo.hasPersistedSession()) {
      state = state.copyWith(status: AuthStatus.unauthenticated);
      return;
    }
    final result = await _repo.refreshSession();
    switch (result) {
      case ApiSuccess(:final data):
        state = state.copyWith(
          status: AuthStatus.authenticated,
          userId: data.userId,
        );
      case ApiFailure():
        state = state.copyWith(status: AuthStatus.unauthenticated);
    }
  }

  Future<void> requestOtp(String phone, String country) async {
    state = state.copyWith(isLoading: true, errorMessage: null);
    final result = await _repo.requestOtp(
      phoneNumber: phone,
      countryCode: country,
    );
    switch (result) {
      case ApiSuccess():
        state = state.copyWith(
          status: AuthStatus.otpSent,
          phoneNumber: phone,
          countryCode: country,
          isLoading: false,
        );
      case ApiFailure(:final message):
        state = state.copyWith(isLoading: false, errorMessage: message);
    }
  }

  Future<void> verifyOtp(String code, DeviceInfoPayload device) async {
    if (state.phoneNumber == null || state.countryCode == null) return;
    state = state.copyWith(isLoading: true, errorMessage: null);
    final result = await _repo.verifyOtp(
      phoneNumber: state.phoneNumber!,
      countryCode: state.countryCode!,
      code: code,
      device: device,
    );
    switch (result) {
      case ApiSuccess(:final data):
        state = state.copyWith(
          status: data.isNewUser
              ? AuthStatus.needsProfileSetup
              : AuthStatus.authenticated,
          userId: data.userId,
          isLoading: false,
        );
      case ApiFailure(:final message):
        state = state.copyWith(isLoading: false, errorMessage: message);
    }
  }

  void completeProfileSetup() =>
      state = state.copyWith(status: AuthStatus.authenticated);
  Future<void> logout() async {
    try {
      await _repo.logout();
    } finally {
      try {
        await _clearLocalSessionData();
      } finally {
        state = const AuthState(status: AuthStatus.unauthenticated);
      }
    }
  }
}

final authControllerProvider = StateNotifierProvider<AuthController, AuthState>(
  (ref) => AuthController(ref.read(authRepositoryProvider)),
);
