import 'dart:async';

import 'package:flutter_test/flutter_test.dart';

import 'package:bharatchat/core/network/websocket_client.dart';
import 'package:bharatchat/features/chat_list/data/datasources/chat_remote_datasource.dart';
import 'package:bharatchat/features/chat_list/domain/entities/chat_summary.dart';
import 'package:bharatchat/features/chat_list/presentation/providers/chat_list_providers.dart';

class FakeWsClient implements WebSocketClientService {
  final _eventsController = StreamController<WsEnvelope>.broadcast();
  final _statusController = StreamController<WsConnectionStatus>.broadcast();

  @override
  Stream<WsEnvelope> get events => _eventsController.stream;

  void emit(WsEnvelope envelope) => _eventsController.add(envelope);
  void emitStatus(WsConnectionStatus status) => _statusController.add(status);

  @override
  void send(WsEnvelope envelope) {}
  @override
  Future<void> connect() async {}
  @override
  void disconnect() {}
  @override
  void dispose() {
    _eventsController.close();
    _statusController.close();
  }

  @override
  WsConnectionStatus get status => WsConnectionStatus.connected;
  @override
  Stream<WsConnectionStatus> get statusStream => _statusController.stream;
}

class FakeChatRemoteDataSource implements ChatRemoteDataSource {
  List<ChatSummary> chatsToReturn = [];
  int listCallCount = 0;

  @override
  Future<List<ChatSummary>> listChats() async {
    listCallCount++;
    return chatsToReturn;
  }

  @override
  Future<String> startDirectChat(String peerUserId) async => 'new-chat-id';

  @override
  Future<void> setMuted(String chatId, bool value) async {}
  @override
  Future<void> setPinned(String chatId, bool value) async {}
  @override
  Future<void> setArchived(String chatId, bool value) async {}
}

ChatSummary _sampleChat({required String id, int unreadCount = 0}) =>
    ChatSummary(
      id: id,
      type: 'direct',
      peerUserId: 'peer-$id',
      peerDisplayName: 'Peer $id',
      peerIsOnline: false,
      unreadCount: unreadCount,
      isMuted: false,
      isPinned: false,
      lastActivityAt: DateTime(2024, 1, 1),
    );

void main() {
  late FakeWsClient fakeWs;
  late FakeChatRemoteDataSource fakeRemote;
  late ChatListNotifier notifier;

  setUp(() {
    fakeWs = FakeWsClient();
    fakeRemote = FakeChatRemoteDataSource();
  });

  tearDown(() {
    notifier.dispose();
    fakeWs.dispose();
  });

  test('loads chats on construction', () async {
    fakeRemote.chatsToReturn = [_sampleChat(id: 'c1')];
    notifier = ChatListNotifier(fakeRemote, fakeWs);
    await Future<void>.delayed(Duration.zero);

    expect(notifier.state.value?.length, 1);
    expect(notifier.state.value?.first.id, 'c1');
  });

  test(
    'new_message for a known chat bumps it to the top and increments unread',
    () async {
      fakeRemote.chatsToReturn = [_sampleChat(id: 'c1'), _sampleChat(id: 'c2')];
      notifier = ChatListNotifier(fakeRemote, fakeWs);
      await Future<void>.delayed(Duration.zero);

      fakeWs.emit(
        WsEnvelope(
          type: WsEventType.newMessage,
          payload: {
            'chatId': 'c2',
            'body': 'new text',
            'type': 'text',
            'senderId': 'peer-c2',
          },
        ),
      );
      await Future<void>.delayed(Duration.zero);

      final chats = notifier.state.value!;
      expect(chats.first.id, 'c2');
      expect(chats.first.unreadCount, 1);
      expect(chats.first.lastMessageBody, 'new text');
    },
  );

  test('new_message for an unknown chat triggers a full refresh', () async {
    fakeRemote.chatsToReturn = [_sampleChat(id: 'c1')];
    notifier = ChatListNotifier(fakeRemote, fakeWs);
    await Future<void>.delayed(Duration.zero);
    final callsBefore = fakeRemote.listCallCount;

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.newMessage,
        payload: {
          'chatId': 'brand-new-chat',
          'body': 'hi',
          'type': 'text',
          'senderId': 'stranger',
        },
      ),
    );
    await Future<void>.delayed(Duration.zero);

    expect(fakeRemote.listCallCount, greaterThan(callsBefore));
  });

  test('presence_update toggles the matching peer online status', () async {
    fakeRemote.chatsToReturn = [_sampleChat(id: 'c1')];
    notifier = ChatListNotifier(fakeRemote, fakeWs);
    await Future<void>.delayed(Duration.zero);

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.presenceUpdate,
        payload: {'userId': 'peer-c1', 'isOnline': true},
      ),
    );
    await Future<void>.delayed(Duration.zero);

    expect(notifier.state.value!.first.peerIsOnline, isTrue);
  });

  test(
    'reconnect refreshes REST state to recover missed pubsub events',
    () async {
      fakeRemote.chatsToReturn = [_sampleChat(id: 'c1')];
      notifier = ChatListNotifier(fakeRemote, fakeWs);
      await Future<void>.delayed(Duration.zero);
      final callsBefore = fakeRemote.listCallCount;

      fakeRemote.chatsToReturn = [_sampleChat(id: 'c1', unreadCount: 3)];
      fakeWs.emitStatus(WsConnectionStatus.connected);
      await Future<void>.delayed(Duration.zero);

      expect(fakeRemote.listCallCount, greaterThan(callsBefore));
      expect(notifier.state.value!.single.unreadCount, 3);
    },
  );
}
