import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:package_info_plus/package_info_plus.dart';

class DeviceInfoPayload {
  final String platform, deviceName, appVersion, installationId;
  final String? pushToken;
  const DeviceInfoPayload({
    required this.platform,
    required this.deviceName,
    required this.appVersion,
    this.installationId = '',
    this.pushToken,
  });
  Map<String, dynamic> toJson() => {
    'platform': platform,
    'deviceName': deviceName,
    'appVersion': appVersion,
    if (installationId.isNotEmpty) 'installationId': installationId,
    if (pushToken != null) 'pushToken': pushToken,
  };
  static Future<DeviceInfoPayload> current({String installationId = ''}) async {
    final info = await PackageInfo.fromPlatform();
    final platform = kIsWeb
        ? 'web'
        : Platform.isAndroid
        ? 'android'
        : Platform.isIOS
        ? 'ios'
        : 'desktop';
    return DeviceInfoPayload(
      platform: platform,
      deviceName: '$platform device',
      appVersion: info.version,
      installationId: installationId,
    );
  }
}
