import 'package:dio/dio.dart';

import '../../domain/entities/chat_message.dart';

class MessageRemoteDataSource {
  final Dio _dio;
  MessageRemoteDataSource(this._dio);
  Future<List<ChatMessage>> getHistory(
    String id, {
    String? beforeMessageId,
  }) async {
    final r = await _dio.get(
      '/chats/$id/messages',
      queryParameters: {if (beforeMessageId != null) 'before': beforeMessageId},
    );
    final items = (r.data as Map)['messages'] as List;
    return items
        .map((e) => ChatMessage.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<void> markChatReadUpTo(String id, String message) =>
      _dio.post('/chats/$id/read', data: {'upToMessageId': message});
}
