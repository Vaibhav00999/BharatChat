import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:uuid/uuid.dart';

class SecureTokenStorage {
  static const _accessTokenKey = 'bharatchat_access_token';
  static const _refreshTokenKey = 'bharatchat_refresh_token';
  static const _installationIdKey = 'bharatchat_installation_id';
  final FlutterSecureStorage _storage;
  SecureTokenStorage({FlutterSecureStorage? storage})
    : _storage =
          storage ??
          const FlutterSecureStorage(
            aOptions: AndroidOptions(encryptedSharedPreferences: true),
          );
  Future<void> saveTokens({
    required String accessToken,
    required String refreshToken,
  }) async {
    await _storage.write(key: _accessTokenKey, value: accessToken);
    await _storage.write(key: _refreshTokenKey, value: refreshToken);
  }

  Future<String?> getAccessToken() => _storage.read(key: _accessTokenKey);
  Future<String?> getRefreshToken() => _storage.read(key: _refreshTokenKey);

  Future<void> writeSecret(String key, String value) =>
      _storage.write(key: key, value: value);
  Future<String?> readSecret(String key) => _storage.read(key: key);
  Future<void> deleteSecret(String key) => _storage.delete(key: key);
  Future<String> getOrCreateInstallationId() async {
    final existing = await _storage.read(key: _installationIdKey);
    if (existing != null && existing.isNotEmpty) return existing;
    final created = const Uuid().v4();
    await _storage.write(key: _installationIdKey, value: created);
    return created;
  }

  Future<void> clear() async {
    await _storage.delete(key: _accessTokenKey);
    await _storage.delete(key: _refreshTokenKey);
  }
}
