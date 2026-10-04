import 'package:dio/dio.dart';

import '../../domain/entities/chat_summary.dart';

class ChatRemoteDataSource {
  final Dio _dio;
  ChatRemoteDataSource(this._dio);
  Future<List<ChatSummary>> listChats() async {
    final r = await _dio.get('/chats');
    final items = (r.data as Map)['chats'] as List;
    return items
        .map((e) => ChatSummary.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<String> startDirectChat(String userId) async {
    final r = await _dio.post('/chats/direct', data: {'peerUserId': userId});
    return (r.data as Map)['id'] as String;
  }

  Future<void> setMuted(String id, bool value) =>
      _dio.patch('/chats/$id/mute', data: {'value': value});
  Future<void> setPinned(String id, bool value) =>
      _dio.patch('/chats/$id/pin', data: {'value': value});
  Future<void> setArchived(String id, bool value) =>
      _dio.patch('/chats/$id/archive', data: {'value': value});
}
