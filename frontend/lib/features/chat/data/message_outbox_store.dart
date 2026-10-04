import '../../../core/storage/hive_boxes.dart';
import '../domain/entities/chat_message.dart';

abstract class MessageOutboxStore {
  Future<List<ChatMessage>> load(String userId, String chatId);
  Future<void> put(String userId, ChatMessage message);
  Future<void> remove(String clientGeneratedId);
  Future<void> clearUser(String userId);
}

/// Used by isolated controller tests and embedders that do not initialize Hive.
/// The application provider always supplies [HiveMessageOutboxStore].
class MemoryMessageOutboxStore implements MessageOutboxStore {
  final Map<String, ({String userId, ChatMessage message})> _records = {};

  @override
  Future<List<ChatMessage>> load(String userId, String chatId) async =>
      _records.values
          .where(
            (record) =>
                record.userId == userId && record.message.chatId == chatId,
          )
          .map((record) => record.message)
          .toList()
        ..sort((a, b) => a.createdAt.compareTo(b.createdAt));

  @override
  Future<void> put(String userId, ChatMessage message) async {
    final clientId = message.clientGeneratedId;
    if (clientId == null || clientId.isEmpty) {
      throw ArgumentError('Outbox messages require a client-generated ID');
    }
    _records[clientId] = (userId: userId, message: message);
  }

  @override
  Future<void> remove(String clientGeneratedId) async {
    _records.remove(clientGeneratedId);
  }

  @override
  Future<void> clearUser(String userId) async {
    _records.removeWhere((_, record) => record.userId == userId);
  }
}

class HiveMessageOutboxStore implements MessageOutboxStore {
  Map<String, dynamic> _encode(String userId, ChatMessage message) => {
    'userId': userId,
    'id': message.id,
    'chatId': message.chatId,
    'senderId': message.senderId,
    'type': message.type,
    'body': message.body,
    'replyToMessageId': message.replyToMessageId,
    'clientGeneratedId': message.clientGeneratedId,
    'ciphertext': message.ciphertext,
    'encryptionProtocol': message.encryptionProtocol,
    'encryptionVersion': message.encryptionVersion,
    'senderDeviceId': message.senderDeviceId,
    'expiresAt': message.expiresAt?.toUtc().toIso8601String(),
    'createdAt': message.createdAt.toUtc().toIso8601String(),
    'status': message.status.name,
  };

  ChatMessage? _decode(Map<dynamic, dynamic> value) {
    try {
      final clientGeneratedId = value['clientGeneratedId'] as String?;
      final createdAt = DateTime.tryParse(value['createdAt'] as String? ?? '');
      final statusName = value['status'] as String?;
      if (clientGeneratedId == null ||
          createdAt == null ||
          statusName == null) {
        return null;
      }
      MessageDeliveryStatus? status;
      for (final candidate in MessageDeliveryStatus.values) {
        if (candidate.name == statusName) {
          status = candidate;
          break;
        }
      }
      if (status == null) return null;
      return ChatMessage(
        id: value['id'] as String? ?? clientGeneratedId,
        chatId: value['chatId'] as String,
        senderId: value['senderId'] as String?,
        type: value['type'] as String,
        body: value['body'] as String?,
        replyToMessageId: value['replyToMessageId'] as String?,
        clientGeneratedId: clientGeneratedId,
        ciphertext: value['ciphertext'] as String?,
        encryptionProtocol: value['encryptionProtocol'] as String?,
        encryptionVersion: value['encryptionVersion'] as int?,
        senderDeviceId: value['senderDeviceId'] as String?,
        expiresAt: value['expiresAt'] == null
            ? null
            : DateTime.tryParse(value['expiresAt'] as String),
        createdAt: createdAt,
        status: status,
      );
    } on Object {
      return null;
    }
  }

  @override
  Future<List<ChatMessage>> load(String userId, String chatId) async {
    final messages = <ChatMessage>[];
    for (final raw in HiveBoxes.messageOutbox.values) {
      if (raw is! Map || raw['userId'] != userId || raw['chatId'] != chatId) {
        continue;
      }
      final message = _decode(raw);
      if (message != null) messages.add(message);
    }
    messages.sort((a, b) => a.createdAt.compareTo(b.createdAt));
    return messages;
  }

  @override
  Future<void> put(String userId, ChatMessage message) async {
    final clientGeneratedId = message.clientGeneratedId;
    if (clientGeneratedId == null || clientGeneratedId.isEmpty) {
      throw ArgumentError('Outbox messages require a client-generated ID');
    }
    await HiveBoxes.messageOutbox.put(
      clientGeneratedId,
      _encode(userId, message),
    );
  }

  @override
  Future<void> remove(String clientGeneratedId) =>
      HiveBoxes.messageOutbox.delete(clientGeneratedId);

  @override
  Future<void> clearUser(String userId) async {
    final keys = <dynamic>[];
    for (final key in HiveBoxes.messageOutbox.keys) {
      final raw = HiveBoxes.messageOutbox.get(key);
      if (raw is Map && raw['userId'] == userId) keys.add(key);
    }
    await HiveBoxes.messageOutbox.deleteAll(keys);
  }
}
