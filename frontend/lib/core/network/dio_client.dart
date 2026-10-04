import 'package:dio/dio.dart';

import '../config/env.dart';
import '../storage/secure_storage.dart';

typedef RefreshTokenCallback = Future<bool> Function();

class DioClientFactory {
  static Dio create({
    required SecureTokenStorage tokenStorage,
    required RefreshTokenCallback onUnauthorized,
  }) {
    final dio = Dio(
      BaseOptions(
        baseUrl: Env.baseUrl,
        connectTimeout: const Duration(seconds: 15),
        receiveTimeout: const Duration(seconds: 15),
        headers: {'Accept': 'application/json'},
      ),
    );
    Future<bool>? refreshInFlight;
    dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (options, handler) async {
          final token = await tokenStorage.getAccessToken();
          if (token != null) {
            options.headers['Authorization'] = 'Bearer $token';
          }
          handler.next(options);
        },
        onError: (error, handler) async {
          final unauthorized = error.response?.statusCode == 401;
          final refreshCall = error.requestOptions.path.contains(
            '/auth/refresh',
          );
          final alreadyRetried =
              error.requestOptions.extra['authRetried'] == true;
          if (!unauthorized || refreshCall || alreadyRetried) {
            handler.next(error);
            return;
          }
          refreshInFlight ??= onUnauthorized().whenComplete(
            () => refreshInFlight = null,
          );
          try {
            final refreshed = await refreshInFlight!;
            if (!refreshed) {
              handler.next(error);
              return;
            }
            final token = await tokenStorage.getAccessToken();
            if (token == null) {
              handler.next(error);
              return;
            }
            final options = error.requestOptions;
            options.headers['Authorization'] = 'Bearer $token';
            options.extra['authRetried'] = true;
            if (options.method == 'DELETE' && options.path == '/users/me') {
              final refreshToken = await tokenStorage.getRefreshToken();
              if (refreshToken == null || refreshToken.isEmpty) {
                handler.next(error);
                return;
              }
              options.data = {
                ...Map<String, dynamic>.from(options.data as Map),
                'refreshToken': refreshToken,
              };
            }
            handler.resolve(await dio.fetch(options));
          } catch (_) {
            handler.next(error);
          }
        },
      ),
    );
    return dio;
  }
}
