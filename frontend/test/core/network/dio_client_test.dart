import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:bharatchat/core/network/dio_client.dart';
import 'package:bharatchat/core/storage/secure_storage.dart';
import 'package:bharatchat/features/privacy/data/privacy_remote_datasource.dart';

class MemoryTokens extends SecureTokenStorage {
  String? access = 'old-access';
  String? refresh = 'old-refresh';
  @override
  Future<String?> getAccessToken() async => access;
  @override
  Future<String?> getRefreshToken() async => refresh;
}

class RecordingAdapter implements HttpClientAdapter {
  final Future<ResponseBody> Function(RequestOptions) respond;
  final requests = <Map<String, dynamic>>[];
  RecordingAdapter(this.respond);
  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? stream,
    Future<void>? cancelFuture,
  ) async {
    requests.add({
      'path': options.path,
      'authorization': options.headers['Authorization'],
      'data': options.data is Map
          ? Map<String, dynamic>.from(options.data as Map)
          : options.data,
    });
    return respond(options);
  }

  @override
  void close({bool force = false}) {}
}

void main() {
  test('deletion retry uses both rotated session credentials', () async {
    final tokens = MemoryTokens();
    var refreshCount = 0;
    final dio = DioClientFactory.create(
      tokenStorage: tokens,
      onUnauthorized: () async {
        refreshCount++;
        tokens.access = 'new-access';
        tokens.refresh = 'new-refresh';
        return true;
      },
    );
    final adapter = RecordingAdapter(
      (options) async => ResponseBody.fromString(
        '{}',
        options.headers['Authorization'] == 'Bearer old-access' ? 401 : 204,
        headers: {
          'content-type': ['application/json'],
        },
      ),
    );
    dio.httpClientAdapter = adapter;
    addTearDown(dio.close);
    await PrivacyRemoteDataSource(dio, tokens).deleteAccount();
    expect(refreshCount, 1);
    expect(adapter.requests, hasLength(2));
    expect(
      (adapter.requests.first['data'] as Map)['refreshToken'],
      'old-refresh',
    );
    expect(adapter.requests.last['authorization'], 'Bearer new-access');
    expect(
      (adapter.requests.last['data'] as Map)['refreshToken'],
      'new-refresh',
    );
  });

  test('failed refresh never retries a destructive request', () async {
    final tokens = MemoryTokens();
    final dio = DioClientFactory.create(
      tokenStorage: tokens,
      onUnauthorized: () async => false,
    );
    final adapter = RecordingAdapter(
      (_) async => ResponseBody.fromString(
        '{}',
        401,
        headers: {
          'content-type': ['application/json'],
        },
      ),
    );
    dio.httpClientAdapter = adapter;
    addTearDown(dio.close);
    await expectLater(
      PrivacyRemoteDataSource(dio, tokens).deleteAccount(),
      throwsA(isA<DioException>()),
    );
    expect(adapter.requests, hasLength(1));
  });

  test('account export accepts only ZIP responses', () async {
    final dio = Dio();
    var valid = true;
    dio.httpClientAdapter = RecordingAdapter(
      (_) async => ResponseBody.fromBytes(
        valid ? [0x50, 0x4b, 3, 4, 0] : utf8.encode('{"error":"no archive"}'),
        200,
        headers: {
          'content-type': [valid ? 'application/zip' : 'application/json'],
        },
      ),
    );
    addTearDown(dio.close);
    final source = PrivacyRemoteDataSource(dio, MemoryTokens());
    expect(await source.exportAccount(), [0x50, 0x4b, 3, 4, 0]);
    valid = false;
    await expectLater(source.exportAccount(), throwsFormatException);
  });
}
