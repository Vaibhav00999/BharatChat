import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/network/websocket_client.dart';
import '../../../../core/config/env.dart';
import '../../../auth/presentation/providers/auth_providers.dart';
import '../../../privacy/presentation/privacy_providers.dart';
import '../../data/datasources/chat_remote_datasource.dart';
import '../../domain/entities/chat_summary.dart';

final chatRemoteDataSourceProvider = Provider(
  (ref) => ChatRemoteDataSource(ref.read(dioProvider)),
);
final webSocketClientProvider = Provider<WebSocketClientService>((ref) {
  final sessionUserId = ref.watch(
    authControllerProvider.select((s) => s.userId),
  );
  final keyManager = ref.read(deviceKeyManagerProvider);
  final c = WebSocketClientService(
    ticketProvider: () async {
      try {
        final userId = ref.read(authControllerProvider).userId;
        if (userId == null || userId.isEmpty) return null;
        if (!Env.allowPlaintextMessaging) {
          await keyManager.ensurePublished(userId);
        }
        final response = await ref.read(dioProvider).post('/auth/ws-ticket');
        return (response.data as Map<String, dynamic>)['ticket'] as String?;
      } catch (_) {
        return null;
      }
    },
  );
  ref.onDispose(c.dispose);
  if (sessionUserId != null) unawaited(c.connect());
  return c;
});

class ChatListNotifier extends StateNotifier<AsyncValue<List<ChatSummary>>> {
  final ChatRemoteDataSource _remote;
  final WebSocketClientService _ws;
  StreamSubscription<WsEnvelope>? _sub;
  StreamSubscription<WsConnectionStatus>? _statusSub;
  ChatListNotifier(this._remote, this._ws) : super(const AsyncValue.loading()) {
    _load();
    _sub = _ws.events.listen(_event);
    _statusSub = _ws.statusStream.listen((status) {
      if (status == WsConnectionStatus.connected) unawaited(refresh());
    });
  }
  Future<void> _load() async {
    try {
      final chats = await _remote.listChats();
      if (mounted) state = AsyncValue.data(chats);
    } catch (e, st) {
      if (mounted) state = AsyncValue.error(e, st);
    }
  }

  Future<void> refresh() => _load();
  void _event(WsEnvelope e) {
    if (e.type == WsEventType.newMessage) {
      _message(e.payload);
    } else if (e.type == WsEventType.presenceUpdate) {
      _presence(e.payload);
    }
  }

  void _message(Map<String, dynamic> p) {
    final id = p['chatId'] as String?;
    final current = state.valueOrNull;
    if (id == null || current == null) return;
    final i = current.indexWhere((c) => c.id == id);
    if (i < 0) {
      unawaited(refresh());
      return;
    }
    final updated = current[i].copyWith(
      lastMessageBody: p['body'] as String?,
      lastMessageType: p['type'] as String?,
      lastMessageSenderId: p['senderId'] as String?,
      unreadCount: current[i].unreadCount + 1,
      lastActivityAt:
          DateTime.tryParse(p['createdAt'] as String? ?? '') ?? DateTime.now(),
    );
    final next = [...current]..removeAt(i);
    next.insert(0, updated);
    state = AsyncValue.data(next);
  }

  void _presence(Map<String, dynamic> p) {
    final id = p['userId'] as String?;
    final online = p['isOnline'] as bool?;
    final current = state.valueOrNull;
    if (id == null || online == null || current == null) return;
    state = AsyncValue.data([
      for (final c in current)
        c.peerUserId == id ? c.copyWith(peerIsOnline: online) : c,
    ]);
  }

  Future<String> startDirectChat(String id) async {
    final chat = await _remote.startDirectChat(id);
    await refresh();
    return chat;
  }

  @override
  void dispose() {
    _sub?.cancel();
    _statusSub?.cancel();
    super.dispose();
  }
}

final chatListProvider =
    StateNotifierProvider.autoDispose<
      ChatListNotifier,
      AsyncValue<List<ChatSummary>>
    >((ref) {
      ref.watch(authControllerProvider.select((s) => s.userId));
      return ChatListNotifier(
        ref.read(chatRemoteDataSourceProvider),
        ref.read(webSocketClientProvider),
      );
    });
