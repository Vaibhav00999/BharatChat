class ChatSummary {
  final String id, type;
  final String? peerUserId,
      peerAvatarUrl,
      groupName,
      groupIconUrl,
      lastMessageBody,
      lastMessageType,
      lastMessageSenderId,
      lastMessageSenderName;
  final String peerDisplayName;
  final bool peerIsOnline, isMuted, isPinned;
  final int unreadCount;
  final DateTime lastActivityAt;
  const ChatSummary({
    required this.id,
    required this.type,
    this.peerUserId,
    required this.peerDisplayName,
    this.peerAvatarUrl,
    required this.peerIsOnline,
    this.groupName,
    this.groupIconUrl,
    this.lastMessageBody,
    this.lastMessageType,
    this.lastMessageSenderId,
    this.lastMessageSenderName,
    required this.unreadCount,
    required this.isMuted,
    required this.isPinned,
    required this.lastActivityAt,
  });
  String get displayName =>
      type == 'group' ? (groupName ?? 'Group') : peerDisplayName;
  String? get displayAvatar => type == 'group' ? groupIconUrl : peerAvatarUrl;
  factory ChatSummary.fromJson(Map<String, dynamic> j) => ChatSummary(
    id: j['id'] as String,
    type: j['type'] as String,
    peerUserId: j['peerUserId'] as String?,
    peerDisplayName: j['peerDisplayName'] as String? ?? '',
    peerAvatarUrl: j['peerAvatarUrl'] as String?,
    peerIsOnline: j['peerIsOnline'] as bool? ?? false,
    groupName: j['groupName'] as String?,
    groupIconUrl: j['groupIconUrl'] as String?,
    lastMessageBody: j['lastMessageBody'] as String?,
    lastMessageType: j['lastMessageType'] as String?,
    lastMessageSenderId: j['lastMessageSenderId'] as String?,
    lastMessageSenderName: j['lastMessageSenderName'] as String?,
    unreadCount: (j['unreadCount'] as num?)?.toInt() ?? 0,
    isMuted: j['isMuted'] as bool? ?? false,
    isPinned: j['isPinned'] as bool? ?? false,
    lastActivityAt: DateTime.parse(j['lastActivityAt'] as String),
  );
  ChatSummary copyWith({
    String? lastMessageBody,
    String? lastMessageType,
    String? lastMessageSenderId,
    int? unreadCount,
    bool? peerIsOnline,
    DateTime? lastActivityAt,
  }) => ChatSummary(
    id: id,
    type: type,
    peerUserId: peerUserId,
    peerDisplayName: peerDisplayName,
    peerAvatarUrl: peerAvatarUrl,
    peerIsOnline: peerIsOnline ?? this.peerIsOnline,
    groupName: groupName,
    groupIconUrl: groupIconUrl,
    lastMessageBody: lastMessageBody ?? this.lastMessageBody,
    lastMessageType: lastMessageType ?? this.lastMessageType,
    lastMessageSenderId: lastMessageSenderId ?? this.lastMessageSenderId,
    lastMessageSenderName: lastMessageSenderName,
    unreadCount: unreadCount ?? this.unreadCount,
    isMuted: isMuted,
    isPinned: isPinned,
    lastActivityAt: lastActivityAt ?? this.lastActivityAt,
  );
}
