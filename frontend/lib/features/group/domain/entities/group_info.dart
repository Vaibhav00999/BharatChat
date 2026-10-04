class GroupInfo {
  final String chatId, name;
  final String? description, iconUrl, inviteCode;
  final bool inviteCodeEnabled, onlyAdminsCanPost, onlyAdminsCanEditInfo;
  final int maxMembers;
  final DateTime createdAt;
  const GroupInfo({
    required this.chatId,
    required this.name,
    this.description,
    this.iconUrl,
    this.inviteCode,
    required this.inviteCodeEnabled,
    required this.onlyAdminsCanPost,
    required this.onlyAdminsCanEditInfo,
    required this.maxMembers,
    required this.createdAt,
  });
  factory GroupInfo.fromJson(Map<String, dynamic> j) => GroupInfo(
    chatId: j['chatId'] as String,
    name: j['name'] as String,
    description: j['description'] as String?,
    iconUrl: j['iconUrl'] as String?,
    inviteCode: j['inviteCode'] as String?,
    inviteCodeEnabled: j['inviteCodeEnabled'] as bool? ?? true,
    onlyAdminsCanPost: j['onlyAdminsCanPost'] as bool? ?? false,
    onlyAdminsCanEditInfo: j['onlyAdminsCanEditInfo'] as bool? ?? true,
    maxMembers: (j['maxMembers'] as num?)?.toInt() ?? 256,
    createdAt: DateTime.parse(j['createdAt'] as String),
  );
}

class GroupMember {
  final String userId, displayName, role;
  final String? avatarUrl;
  final bool isOnline;
  final DateTime joinedAt;
  const GroupMember({
    required this.userId,
    required this.displayName,
    required this.role,
    this.avatarUrl,
    required this.isOnline,
    required this.joinedAt,
  });
  factory GroupMember.fromJson(Map<String, dynamic> j) => GroupMember(
    userId: j['userId'] as String,
    displayName: j['displayName'] as String,
    role: j['role'] as String,
    avatarUrl: j['avatarUrl'] as String?,
    isOnline: j['isOnline'] as bool? ?? false,
    joinedAt: DateTime.parse(j['joinedAt'] as String),
  );
}
