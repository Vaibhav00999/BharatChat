import 'dart:async';

import 'package:flutter_test/flutter_test.dart';

import 'package:bharatchat/core/network/websocket_client.dart';
import 'package:bharatchat/features/chat/data/datasources/message_remote_datasource.dart';
import 'package:bharatchat/features/chat/data/message_outbox_store.dart';
import 'package:bharatchat/features/chat/domain/entities/chat_message.dart';
import 'package:bharatchat/features/chat/presentation/providers/chat_providers.dart';

/// A fake WebSocketClientService that lets tests inject inbound envelopes and
/// capture outbound sends, without any real socket — this is the standard pattern
/// used throughout this project's test doubles (see Module 2's fake repositories).
class FakeWsClient implements WebSocketClientService {
  final _eventsController = StreamController<WsEnvelope>.broadcast();
  final _statusController = StreamController<WsConnectionStatus>.broadcast();
  final List<WsEnvelope> sent = [];
  WsConnectionStatus currentStatus = WsConnectionStatus.connected;

  @override
  Stream<WsEnvelope> get events => _eventsController.stream;

  void emit(WsEnvelope envelope) => _eventsController.add(envelope);
  void emitStatus(WsConnectionStatus status) {
    currentStatus = status;
    _statusController.add(status);
  }

  @override
  void send(WsEnvelope envelope) => sent.add(envelope);

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
  WsConnectionStatus get status => currentStatus;

  @override
  Stream<WsConnectionStatus> get statusStream => _statusController.stream;
}

class FakeMessageRemoteDataSource implements MessageRemoteDataSource {
  List<ChatMessage> historyToReturn = [];
  bool markReadCalled = false;
  bool failHistory = false;

  @override
  Future<List<ChatMessage>> getHistory(
    String chatId, {
    String? beforeMessageId,
  }) async {
    if (failHistory) throw Exception('offline');
    return historyToReturn;
  }

  @override
  Future<void> markChatReadUpTo(String chatId, String upToMessageId) async {
    markReadCalled = true;
  }
}

void main() {
  late FakeWsClient fakeWs;
  late FakeMessageRemoteDataSource fakeRemote;
  late ChatController controller;

  setUp(() {
    fakeWs = FakeWsClient();
    fakeRemote = FakeMessageRemoteDataSource();
  });

  tearDown(() {
    controller.dispose();
    fakeWs.dispose();
  });

  test(
    'loads initial history in oldest-first order and marks it read',
    () async {
      fakeRemote.historyToReturn = [
        ChatMessage(
          id: 'm2',
          chatId: 'chat-1',
          senderId: 'peer',
          type: 'text',
          body: 'second',
          createdAt: DateTime(2024, 1, 1, 10, 1),
          status: MessageDeliveryStatus.sent,
        ),
        ChatMessage(
          id: 'm1',
          chatId: 'chat-1',
          senderId: 'peer',
          type: 'text',
          body: 'first',
          createdAt: DateTime(2024, 1, 1, 10, 0),
          status: MessageDeliveryStatus.sent,
        ),
      ];

      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        allowPlaintextMessaging: true,
      );
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.messages.map((m) => m.id).toList(), ['m1', 'm2']);
      expect(fakeRemote.markReadCalled, isTrue);
    },
  );

  test('sendMessage adds an optimistic message and sends over WS', () async {
    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      allowPlaintextMessaging: true,
    );
    await Future<void>.delayed(Duration.zero);

    controller.sendMessage('hello world');
    await Future<void>.delayed(Duration.zero);

    expect(controller.state.messages.length, 1);
    expect(
      controller.state.messages.first.status,
      MessageDeliveryStatus.sending,
    );
    expect(controller.state.messages.first.body, 'hello world');

    expect(fakeWs.sent.length, 1);
    expect(fakeWs.sent.first.type, WsEventType.sendMessage);
    expect(fakeWs.sent.first.payload['body'], 'hello world');
  });

  test('plaintext-disabled builds refuse message and typing events', () async {
    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      allowPlaintextMessaging: false,
    );
    await Future<void>.delayed(Duration.zero);

    controller.sendMessage('must not leave this device');
    controller.notifyTypingStart();

    expect(controller.state.messages, isEmpty);
    expect(controller.state.canSendMessages, isFalse);
    expect(controller.state.errorMessage, isNotNull);
    expect(fakeWs.sent, isEmpty);
  });

  test(
    'correlated send errors fail only that message; retries retain its ID',
    () async {
      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        allowPlaintextMessaging: true,
      );
      await Future<void>.delayed(Duration.zero);
      controller.sendMessage('hello');
      await Future<void>.delayed(Duration.zero);
      final original = fakeWs.sent.single;
      expect(original.requestId, original.payload['clientGeneratedId']);
      fakeWs.emit(
        WsEnvelope(
          type: WsEventType.error,
          requestId: 'unrelated',
          payload: {'code': 'FORBIDDEN'},
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(
        controller.state.messages.single.status,
        MessageDeliveryStatus.sending,
      );
      fakeWs.emit(
        WsEnvelope(
          type: WsEventType.error,
          requestId: original.requestId,
          payload: {'code': 'FORBIDDEN'},
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(
        controller.state.messages.single.status,
        MessageDeliveryStatus.failed,
      );
      expect(controller.state.errorMessage, 'This conversation is unavailable');
      controller.retryMessage(controller.state.messages.single.id);
      await Future<void>.delayed(Duration.zero);
      expect(fakeWs.sent.last.requestId, original.requestId);
      expect(controller.state.messages, hasLength(1));
      expect(
        controller.state.messages.single.status,
        MessageDeliveryStatus.sending,
      );
    },
  );

  test(
    'message_ack replaces the optimistic message with the server-confirmed one',
    () async {
      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        allowPlaintextMessaging: true,
      );
      await Future<void>.delayed(Duration.zero);

      controller.sendMessage('hello');
      await Future<void>.delayed(Duration.zero);
      final clientGeneratedId =
          fakeWs.sent.first.payload['clientGeneratedId'] as String;

      fakeWs.emit(
        WsEnvelope(
          type: WsEventType.messageAck,
          payload: {
            'clientGeneratedId': clientGeneratedId,
            'message': {
              'id': 'server-msg-1',
              'chatId': 'chat-1',
              'senderId': 'me',
              'type': 'text',
              'body': 'hello',
              'createdAt': DateTime.now().toIso8601String(),
            },
          },
        ),
      );
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.messages.length, 1);
      expect(controller.state.messages.first.id, 'server-msg-1');
      expect(
        controller.state.messages.first.status,
        MessageDeliveryStatus.sent,
      );
    },
  );

  test(
    'incoming new_message triggers delivered and read acks back over WS',
    () async {
      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        allowPlaintextMessaging: true,
      );
      await Future<void>.delayed(Duration.zero);

      fakeWs.emit(
        WsEnvelope(
          type: WsEventType.newMessage,
          payload: {
            'id': 'incoming-1',
            'chatId': 'chat-1',
            'senderId': 'peer',
            'type': 'text',
            'body': 'hi there',
            'createdAt': DateTime.now().toIso8601String(),
          },
        ),
      );
      await Future<void>.delayed(Duration.zero);

      expect(controller.state.messages.length, 1);
      expect(controller.state.messages.first.body, 'hi there');

      final sentTypes = fakeWs.sent.map((e) => e.type).toList();
      expect(
        sentTypes,
        containsAll([WsEventType.messageDelivered, WsEventType.messageRead]),
      );
    },
  );

  test('delivery_ack then read_ack never downgrades status', () async {
    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      allowPlaintextMessaging: true,
    );
    await Future<void>.delayed(Duration.zero);

    controller.sendMessage('ping');
    await Future<void>.delayed(Duration.zero);
    final clientGeneratedId =
        fakeWs.sent.first.payload['clientGeneratedId'] as String;

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.messageAck,
        payload: {
          'clientGeneratedId': clientGeneratedId,
          'message': {
            'id': 'server-msg-2',
            'chatId': 'chat-1',
            'senderId': 'me',
            'type': 'text',
            'body': 'ping',
            'createdAt': DateTime.now().toIso8601String(),
          },
        },
      ),
    );
    await Future<void>.delayed(Duration.zero);

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.readAck,
        payload: {'messageId': 'server-msg-2'},
      ),
    );
    await Future<void>.delayed(Duration.zero);
    expect(controller.state.messages.first.status, MessageDeliveryStatus.read);

    // A late delivery_ack arriving after read_ack must not downgrade the status.
    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.deliveryAck,
        payload: {'messageId': 'server-msg-2'},
      ),
    );
    await Future<void>.delayed(Duration.zero);
    expect(controller.state.messages.first.status, MessageDeliveryStatus.read);
  });

  test('typing_update for this chat toggles peerIsTyping', () async {
    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      allowPlaintextMessaging: true,
    );
    await Future<void>.delayed(Duration.zero);

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.typingUpdate,
        payload: {'chatId': 'chat-1', 'isTyping': true},
      ),
    );
    await Future<void>.delayed(Duration.zero);
    expect(controller.state.peerIsTyping, isTrue);

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.typingUpdate,
        payload: {'chatId': 'chat-1', 'isTyping': false},
      ),
    );
    await Future<void>.delayed(Duration.zero);
    expect(controller.state.peerIsTyping, isFalse);
  });

  test('typing_update for a DIFFERENT chat is ignored', () async {
    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      allowPlaintextMessaging: true,
    );
    await Future<void>.delayed(Duration.zero);

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.typingUpdate,
        payload: {'chatId': 'some-other-chat', 'isTyping': true},
      ),
    );
    await Future<void>.delayed(Duration.zero);

    expect(controller.state.peerIsTyping, isFalse);
  });

  test(
    'offline sends persist and flush with the same ID after reconnect',
    () async {
      final outbox = MemoryMessageOutboxStore();
      fakeWs.currentStatus = WsConnectionStatus.disconnected;
      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        outbox: outbox,
        allowPlaintextMessaging: true,
      );
      await Future<void>.delayed(Duration.zero);

      controller.sendMessage('stored offline');
      await Future<void>.delayed(Duration.zero);

      final stored = await outbox.load('me', 'chat-1');
      expect(stored, hasLength(1));
      expect(
        controller.state.messages.single.status,
        MessageDeliveryStatus.queued,
      );
      expect(fakeWs.sent, isEmpty);
      final clientId = stored.single.clientGeneratedId;

      fakeWs.emitStatus(WsConnectionStatus.connected);
      await Future<void>.delayed(Duration.zero);

      expect(fakeWs.sent, hasLength(1));
      expect(fakeWs.sent.single.requestId, clientId);
      expect(fakeWs.sent.single.payload['clientGeneratedId'], clientId);
      expect(
        controller.state.messages.single.status,
        MessageDeliveryStatus.sending,
      );
    },
  );

  test('message acknowledgement removes its encrypted outbox record', () async {
    final outbox = MemoryMessageOutboxStore();
    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      outbox: outbox,
      allowPlaintextMessaging: true,
    );
    await Future<void>.delayed(Duration.zero);
    controller.sendMessage('persist until ack');
    await Future<void>.delayed(Duration.zero);
    final clientId = fakeWs.sent.single.requestId!;

    fakeWs.emit(
      WsEnvelope(
        type: WsEventType.messageAck,
        payload: {
          'clientGeneratedId': clientId,
          'message': {
            'id': 'server-id',
            'chatId': 'chat-1',
            'senderId': 'me',
            'type': 'text',
            'body': 'persist until ack',
            'clientGeneratedId': clientId,
            'createdAt': DateTime.now().toUtc().toIso8601String(),
          },
        },
      ),
    );
    await Future<void>.delayed(Duration.zero);

    expect(await outbox.load('me', 'chat-1'), isEmpty);
    expect(controller.state.messages.single.id, 'server-id');
  });

  test(
    'restored pending message is resent idempotently after process restart',
    () async {
      final outbox = MemoryMessageOutboxStore();
      final createdAt = DateTime.utc(2026, 1, 2, 3, 4);
      await outbox.put(
        'me',
        ChatMessage(
          id: 'client-restart-id',
          chatId: 'chat-1',
          senderId: 'me',
          type: 'text',
          body: 'survives restart',
          clientGeneratedId: 'client-restart-id',
          createdAt: createdAt,
          status: MessageDeliveryStatus.sending,
        ),
      );
      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        outbox: outbox,
        allowPlaintextMessaging: true,
      );
      await Future<void>.delayed(Duration.zero);
      await Future<void>.delayed(Duration.zero);

      expect(
        controller.state.messages.single.clientGeneratedId,
        'client-restart-id',
      );
      expect(fakeWs.sent.single.requestId, 'client-restart-id');
      expect(fakeWs.sent.single.payload['body'], 'survives restart');
    },
  );

  test('restores the local outbox when REST history is unavailable', () async {
    final outbox = MemoryMessageOutboxStore();
    fakeRemote.failHistory = true;
    fakeWs.currentStatus = WsConnectionStatus.disconnected;
    await outbox.put(
      'me',
      ChatMessage(
        id: 'offline-client-id',
        chatId: 'chat-1',
        senderId: 'me',
        type: 'text',
        body: 'available without the server',
        clientGeneratedId: 'offline-client-id',
        createdAt: DateTime.utc(2026, 1, 2, 3, 4),
        status: MessageDeliveryStatus.queued,
      ),
    );

    controller = ChatController(
      chatId: 'chat-1',
      currentUserId: 'me',
      remote: fakeRemote,
      ws: fakeWs,
      outbox: outbox,
      allowPlaintextMessaging: true,
    );
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(Duration.zero);

    expect(controller.state.messages, hasLength(1));
    expect(
      controller.state.messages.single.body,
      'available without the server',
    );
    expect(
      controller.state.messages.single.status,
      MessageDeliveryStatus.queued,
    );
    expect(controller.state.errorMessage, 'Could not load messages');
  });

  test(
    'release lock also blocks restored outbox sends and reconnects',
    () async {
      final outbox = MemoryMessageOutboxStore();
      await outbox.put(
        'me',
        ChatMessage(
          id: 'old-client-id',
          chatId: 'chat-1',
          senderId: 'me',
          type: 'text',
          body: 'must stay local',
          clientGeneratedId: 'old-client-id',
          createdAt: DateTime.utc(2026, 1, 1),
          status: MessageDeliveryStatus.sending,
        ),
      );
      controller = ChatController(
        chatId: 'chat-1',
        currentUserId: 'me',
        remote: fakeRemote,
        ws: fakeWs,
        outbox: outbox,
        allowPlaintextMessaging: false,
      );
      await Future<void>.delayed(Duration.zero);
      fakeWs.emitStatus(WsConnectionStatus.disconnected);
      fakeWs.emitStatus(WsConnectionStatus.connected);
      await Future<void>.delayed(Duration.zero);
      expect(
        controller.state.messages.single.status,
        MessageDeliveryStatus.queued,
      );
      expect(
        fakeWs.sent.where((event) => event.type == WsEventType.sendMessage),
        isEmpty,
      );
      expect(await outbox.load('me', 'chat-1'), hasLength(1));
    },
  );
}
