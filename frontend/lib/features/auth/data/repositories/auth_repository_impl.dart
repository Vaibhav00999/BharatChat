import 'package:dio/dio.dart';

import '../../../../core/network/api_result.dart';
import '../../../../core/storage/secure_storage.dart';
import '../../domain/entities/auth_session.dart';
import '../../domain/entities/device_info.dart';
import '../../domain/repositories/auth_repository.dart';
import '../datasources/auth_remote_datasource.dart';

class AuthRepositoryImpl implements AuthRepository {
  final AuthRemoteDataSource _remote;
  final SecureTokenStorage _storage;
  AuthRepositoryImpl(this._remote, this._storage);
  ApiFailure<T> _error<T>(Object e) {
    if (e is DioException && e.response?.data is Map) {
      final d = e.response!.data as Map;
      return ApiFailure(
        code: d['code'] as String? ?? 'UNKNOWN_ERROR',
        message: d['message'] as String? ?? 'Something went wrong',
      );
    }
    return const ApiFailure(
      code: 'NETWORK_ERROR',
      message: 'Could not reach the server. Please check your connection.',
    );
  }

  @override
  Future<ApiResult<void>> requestOtp({
    required String phoneNumber,
    required String countryCode,
  }) async {
    try {
      await _remote.requestOtp(
        phoneNumber: phoneNumber,
        countryCode: countryCode,
      );
      return const ApiSuccess(null);
    } catch (e) {
      return _error(e);
    }
  }

  @override
  Future<ApiResult<AuthSession>> verifyOtp({
    required String phoneNumber,
    required String countryCode,
    required String code,
    required DeviceInfoPayload device,
  }) async {
    try {
      final s = await _remote.verifyOtp(
        phoneNumber: phoneNumber,
        countryCode: countryCode,
        code: code,
        device: device,
      );
      await _storage.saveTokens(
        accessToken: s.accessToken,
        refreshToken: s.refreshToken,
      );
      return ApiSuccess(s);
    } catch (e) {
      return _error(e);
    }
  }

  @override
  Future<ApiResult<AuthSession>> refreshSession() async {
    try {
      final token = await _storage.getRefreshToken();
      if (token == null) {
        return const ApiFailure(
          code: 'NO_SESSION',
          message: 'No active session',
        );
      }
      final s = await _remote.refresh(refreshToken: token);
      await _storage.saveTokens(
        accessToken: s.accessToken,
        refreshToken: s.refreshToken,
      );
      return ApiSuccess(s);
    } catch (e) {
      await _storage.clear();
      return _error(e);
    }
  }

  @override
  Future<ApiResult<void>> logout() async {
    try {
      await _remote.logout();
    } catch (_) {}
    await _storage.clear();
    return const ApiSuccess(null);
  }

  @override
  Future<bool> hasPersistedSession() async =>
      (await _storage.getRefreshToken()) != null;
}
