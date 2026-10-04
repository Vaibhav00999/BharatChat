import 'dart:typed_data';

import 'package:dio/dio.dart';

import '../../../core/storage/secure_storage.dart';
import '../domain/privacy_dashboard.dart';

class PrivacyRemoteDataSource {
  final Dio _dio;
  final SecureTokenStorage _tokenStorage;

  PrivacyRemoteDataSource(this._dio, this._tokenStorage);

  Future<PrivacyDashboard> load() async {
    final responses = await Future.wait([
      _dio.get('/users/me'),
      _dio.get('/privacy/devices'),
    ]);
    final user = Map<String, dynamic>.from(responses[0].data as Map);
    final devicePayload = Map<String, dynamic>.from(responses[1].data as Map);
    final devices = (devicePayload['devices'] as List)
        .map(
          (item) =>
              LinkedDevice.fromJson(Map<String, dynamic>.from(item as Map)),
        )
        .toList();
    return PrivacyDashboard(
      settings: PrivacySettings.fromJson(user),
      devices: devices,
    );
  }

  Future<void> update(Map<String, dynamic> values) =>
      _dio.patch('/users/me/privacy', data: values);

  Future<void> revokeDevice(String deviceId) =>
      _dio.delete('/privacy/devices/$deviceId');

  Future<Uint8List> exportAccount({CancelToken? cancelToken}) async {
    final response = await _dio.get<List<int>>(
      '/users/me/export',
      cancelToken: cancelToken,
      options: Options(
        responseType: ResponseType.bytes,
        receiveTimeout: const Duration(seconds: 30),
        headers: {'Accept': 'application/zip'},
      ),
    );
    final bytes = response.data;
    if (bytes == null ||
        bytes.length < 4 ||
        bytes.length > 32 * 1024 * 1024 ||
        bytes[0] != 0x50 ||
        bytes[1] != 0x4b ||
        bytes[2] != 3 ||
        bytes[3] != 4 ||
        response.headers.value('content-type')?.split(';').first !=
            'application/zip') {
      throw const FormatException('Invalid account archive');
    }
    return Uint8List.fromList(bytes);
  }

  Future<void> deleteAccount() async {
    final refreshToken = await _tokenStorage.getRefreshToken();
    if (refreshToken == null || refreshToken.isEmpty) {
      throw StateError('Current session confirmation is unavailable');
    }
    await _dio.delete(
      '/users/me',
      data: {'confirmation': 'DELETE', 'refreshToken': refreshToken},
    );
  }
}
