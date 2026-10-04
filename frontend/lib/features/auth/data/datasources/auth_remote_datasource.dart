import 'package:dio/dio.dart';

import '../../domain/entities/auth_session.dart';
import '../../domain/entities/device_info.dart';

class AuthRemoteDataSource {
  final Dio _dio;
  AuthRemoteDataSource(this._dio);
  Future<void> requestOtp({
    required String phoneNumber,
    required String countryCode,
  }) => _dio.post(
    '/auth/otp/request',
    data: {'phoneNumber': phoneNumber, 'countryCode': countryCode},
  );
  Future<AuthSession> verifyOtp({
    required String phoneNumber,
    required String countryCode,
    required String code,
    required DeviceInfoPayload device,
  }) async {
    final r = await _dio.post(
      '/auth/otp/verify',
      data: {
        'phoneNumber': phoneNumber,
        'countryCode': countryCode,
        'code': code,
        'device': device.toJson(),
      },
    );
    return AuthSession.fromJson(Map<String, dynamic>.from(r.data as Map));
  }

  Future<AuthSession> refresh({required String refreshToken}) async {
    final r = await _dio.post(
      '/auth/refresh',
      data: {'refreshToken': refreshToken},
    );
    return AuthSession.fromJson(Map<String, dynamic>.from(r.data as Map));
  }

  Future<void> logout() => _dio.post('/auth/logout');
}
