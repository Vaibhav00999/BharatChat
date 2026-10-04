class PrivacySettings {
  final String lastSeen;
  final String avatar;
  final String about;
  final String phone;
  final String allowGroupAdds;
  final bool readReceipts;
  final bool discoverableByPhone;
  final bool securityNotifications;
  final bool shareTypingIndicators;

  const PrivacySettings({
    required this.lastSeen,
    required this.avatar,
    required this.about,
    required this.phone,
    required this.allowGroupAdds,
    required this.readReceipts,
    required this.discoverableByPhone,
    required this.securityNotifications,
    required this.shareTypingIndicators,
  });

  factory PrivacySettings.fromJson(Map<String, dynamic> json) =>
      PrivacySettings(
        lastSeen: json['privacyLastSeen'] as String,
        avatar: json['privacyAvatar'] as String,
        about: json['privacyAbout'] as String,
        phone: json['privacyPhone'] as String,
        allowGroupAdds: json['allowGroupAdds'] as String,
        readReceipts: json['privacyReadReceipts'] as bool,
        discoverableByPhone: json['discoverableByPhone'] as bool,
        securityNotifications: json['securityNotifications'] as bool,
        shareTypingIndicators: json['shareTypingIndicators'] as bool? ?? false,
      );
}

class LinkedDevice {
  final String deviceId;
  final String platform;
  final String? deviceName;
  final DateTime lastActiveAt;
  final bool hasKeyBundle;
  final bool isCurrent;

  const LinkedDevice({
    required this.deviceId,
    required this.platform,
    this.deviceName,
    required this.lastActiveAt,
    required this.hasKeyBundle,
    required this.isCurrent,
  });

  factory LinkedDevice.fromJson(Map<String, dynamic> json) => LinkedDevice(
    deviceId: json['deviceId'] as String,
    platform: json['platform'] as String,
    deviceName: json['deviceName'] as String?,
    lastActiveAt: DateTime.parse(json['lastActiveAt'] as String),
    hasKeyBundle: json['hasKeyBundle'] as bool? ?? false,
    isCurrent: json['isCurrent'] as bool? ?? false,
  );
}

class PrivacyDashboard {
  final PrivacySettings settings;
  final List<LinkedDevice> devices;

  const PrivacyDashboard({required this.settings, required this.devices});
}
