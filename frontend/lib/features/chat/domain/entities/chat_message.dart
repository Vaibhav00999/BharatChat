enum MessageDeliveryStatus { queued, sending, sent, delivered, read, failed }

class ChatMessage {
  final String id, chatId, type;
  final String? senderId, body, replyToMessageId, clientGeneratedId;
  final String? ciphertext, encryptionProtocol, senderDeviceId;
  final int? encryptionVersion;
  final DateTime? expiresAt;
  final DateTime createdAt;
  final MessageDeliveryStatus status;
  const ChatMessage({
    required this.id,
    required this.chatId,
    required this.senderId,
    required this.type,
    required this.body,
    this.replyToMessageId,
    this.clientGeneratedId,
    this.ciphertext,
    this.encryptionProtocol,
    this.encryptionVersion,
    this.senderDeviceId,
    this.expiresAt,
    required this.createdAt,
    required this.status,
  });
  factory ChatMessage.fromJson(
    Map<String, dynamic> j, {
    MessageDeliveryStatus status = MessageDeliveryStatus.sent,
  }) => ChatMessage(
    id: j['id'] as String,
    chatId: j['chatId'] as String,
    senderId: j['senderId'] as String?,
    type: j['type'] as String,
    body: j['body'] as String?,
    replyToMessageId: j['replyToMessageId'] as String?,
    clientGeneratedId: j['clientGeneratedId'] as String?,
    ciphertext: j['ciphertext'] as String?,
    encryptionProtocol: j['encryptionProtocol'] as String?,
    encryptionVersion: j['encryptionVersion'] as int?,
    senderDeviceId: j['senderDeviceId'] as String?,
    expiresAt: j['expiresAt'] == null
        ? null
        : DateTime.parse(j['expiresAt'] as String),
    createdAt: DateTime.parse(j['createdAt'] as String),
    status: status,
  );
  ChatMessage copyWith({String? id, MessageDeliveryStatus? status}) =>
      ChatMessage(
        id: id ?? this.id,
        chatId: chatId,
        senderId: senderId,
        type: type,
        body: body,
        replyToMessageId: replyToMessageId,
        clientGeneratedId: clientGeneratedId,
        ciphertext: ciphertext,
        encryptionProtocol: encryptionProtocol,
        encryptionVersion: encryptionVersion,
        senderDeviceId: senderDeviceId,
        expiresAt: expiresAt,
        createdAt: createdAt,
        status: status ?? this.status,
      );
}
