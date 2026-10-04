import 'dart:typed_data';
import 'dart:ui' show Rect;

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../auth/presentation/providers/auth_providers.dart';
import '../data/privacy_remote_datasource.dart';
import '../data/device_key_manager.dart';
import '../domain/privacy_dashboard.dart';
import '../data/account_export_file.dart';

typedef AccountExportSaver =
    Future<bool> Function(Uint8List bytes, String filename, Rect origin);

final accountExportSaverProvider = Provider<AccountExportSaver>(
  (ref) => saveAccountExport,
);

final privacyRemoteDataSourceProvider = Provider(
  (ref) => PrivacyRemoteDataSource(
    ref.read(dioProvider),
    ref.read(secureTokenStorageProvider),
  ),
);
final deviceKeyManagerProvider = Provider(
  (ref) => DeviceKeyManager(
    ref.read(dioProvider),
    ref.read(secureTokenStorageProvider),
  ),
);

class PrivacyController extends StateNotifier<AsyncValue<PrivacyDashboard>> {
  final PrivacyRemoteDataSource _remote;

  PrivacyController(this._remote) : super(const AsyncValue.loading()) {
    refresh();
  }

  Future<void> refresh() async {
    try {
      final dashboard = await _remote.load();
      if (mounted) state = AsyncValue.data(dashboard);
    } catch (error, stackTrace) {
      if (mounted) state = AsyncValue.error(error, stackTrace);
    }
  }

  Future<void> update(Map<String, dynamic> values) async {
    await _remote.update(values);
    await refresh();
  }

  Future<void> revokeDevice(String deviceId) async {
    await _remote.revokeDevice(deviceId);
    await refresh();
  }

  Future<void> deleteAccount() => _remote.deleteAccount();
}

final privacyControllerProvider =
    StateNotifierProvider.autoDispose<
      PrivacyController,
      AsyncValue<PrivacyDashboard>
    >((ref) => PrivacyController(ref.read(privacyRemoteDataSourceProvider)));
