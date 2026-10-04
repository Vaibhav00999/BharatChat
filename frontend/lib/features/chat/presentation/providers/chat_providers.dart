// lib/features/chat/presentation/providers/chat_providers.dart
import 'dart:async';

import 'package:uuid/uuid.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/network/websocket_client.dart';
import '../../../../core/config/env.dart';
import '../../../auth/presentation/providers/auth_providers.dart';
import '../../../chat_list/presentation/providers/chat_list_providers.dart';
import '../../data/datasources/message_remote_datasource.dart';
import '../../data/message_outbox_store.dart';
import '../../domain/entities/chat_message.dart';

final messageRemoteDataSourceProvider = Provider(
  (ref) => MessageRemoteDataSource(ref.read(dioProvider)),
);
final messageOutboxStoreProvider = Provider<MessageOutboxStore>(
  (ref) => HiveMessageOutboxStore(),
);

const _uuid = Uuid();

class ChatState {
  final List<ChatMessage> messages; // newest last, for a bottom-anchored list
  final bool isLoadingHistory;
  final bool hasMoreHistory;
  final bool peerIsTyping;
  final bool canSendMessages;
  final String? errorMessage;

  const ChatState({
    this.messages = const [],
    this.isLoadingHistory = true,
    this.hasMoreHistory = true,
    this.peerIsTyping = false,
    this.canSendMessages = false,
    this.errorMessage,
  });

  ChatState copyWith({
    List<ChatMessage>? messages,
    bool? isLoadingHistory,
    bool? hasMoreHistory,
    bool? peerIsTyping,
    bool? canSendMessages,
    String? errorMessage,
  }) {
    return ChatState(
      messages: messages ?? this.messages,
      isLoadingHistory: isLoadingHistory ?? this.isLoadingHistory,
      hasMoreHistory: hasMoreHistory ?? this.hasMoreHistory,
      peerIsTyping: peerIsTyping ?? this.peerIsTyping,
      canSendMessages: canSendMessages ?? this.canSendMessages,
      errorMessage: errorMessage,
    );
  }
}

class ChatController extends StateNotifier<ChatState> {
  final String chatId;
  final String currentUserId;
  final MessageRemoteDataSource _remote;
  final WebSocketClientService _ws;
  final MessageOutboxStore _outbox;
  final bool _allowPlaintextMessaging;

  StreamSubscription<WsEnvelope>? _wsSubscription;
  StreamSubscription<WsConnectionStatus>? _statusSubscription;
  Timer? _typingStopTimer;
  final Map<String, Timer> _ackTimers = {};
  bool _isLoadingMore = false;
  bool _initialized = false;
  bool _flushingOutbox = false;

  ChatController({
    required this.chatId,
    required this.currentUserId,
    required MessageRemoteDataSource remote,
    required WebSocketClientService ws,
    MessageOutboxStore? outbox,
    required bool allowPlaintextMessaging,
  }) : _remote = remote,
       _ws = ws,
       _outbox = outbox ?? MemoryMessageOutboxStore(),
       _allowPlaintextMessaging = allowPlaintextMessaging,
       super(ChatState(canSendMessages: allowPlaintextMessaging)) {
    _subscribeToWsEvents();
    _statusSubscription = _ws.statusStream.listen(_handleConnectionStatus);
    _initialize();
  }

  Future<void> _initialize() async {
    var history = <ChatMessage>[];
    var historyLoaded = false;
    var historyFailed = false;

    try {
      history = await _remote.getHistory(chatId);
      historyLoaded = true;
    } catch (_) {
      historyFailed = true;
    }

    try {
      final stored = await _outbox.load(currentUserId, chatId);
      if (!mounted) return;
      final oldestFirst = history.reversed.toList();
      final confirmedClientIds = history
          .map((message) => message.clientGeneratedId)
          .whereType<String>()
          .toSet();
      final pending = <ChatMessage>[];
      for (final message in stored) {
        final clientId = message.clientGeneratedId!;
        if (confirmedClientIds.contains(clientId)) {
          unawaited(_removeBestEffort(clientId));
          continue;
        }
        pending.add(
          message.status == MessageDeliveryStatus.sending
              ? message.copyWith(status: MessageDeliveryStatus.queued)
              : message,
        );
      }

      // Preserve messages queued while REST history was loading.
      final pendingByClientId = <String, ChatMessage>{
        for (final message in pending) message.clientGeneratedId!: message,
        for (final message in state.messages)
          if (message.clientGeneratedId != null &&
              !confirmedClientIds.contains(message.clientGeneratedId))
            message.clientGeneratedId!: message,
      };
      final merged = [...oldestFirst, ...pendingByClientId.values]
        ..sort((a, b) => a.createdAt.compareTo(b.createdAt));
      state = state.copyWith(
        messages: merged,
        isLoadingHistory: false,
        hasMoreHistory: historyLoaded && history.length >= 50,
        errorMessage: historyFailed ? 'Could not load messages' : null,
      );
      _initialized = true;

      if (historyLoaded && oldestFirst.isNotEmpty) {
        try {
          await _remote.markChatReadUpTo(chatId, oldestFirst.last.id);
        } catch (_) {
          // History is already usable; a later open/reconnect can retry the read
          // watermark without hiding messages or mislabeling outbox recovery.
        }
      }
      if (_ws.status == WsConnectionStatus.connected) {
        unawaited(_flushQueuedMessages());
      }
    } catch (e) {
      if (!mounted) return;
      _initialized = true;
      state = state.copyWith(
        isLoadingHistory: false,
        errorMessage: historyFailed
            ? 'Could not load messages or recover pending messages'
            : 'Could not recover pending messages',
      );
    }
  }

  Future<void> loadMoreHistory() async {
    if (_isLoadingMore || !state.hasMoreHistory || state.messages.isEmpty) {
      return;
    }

    final oldestLoadedId = state.messages.first.id;
    _isLoadingMore = true;
    try {
      final older = await _remote.getHistory(
        chatId,
        beforeMessageId: oldestLoadedId,
      );
      if (!mounted) return;
      if (older.isEmpty) {
        state = state.copyWith(hasMoreHistory: false);
        return;
      }
      final oldestFirst = older.reversed.toList();
      state = state.copyWith(
        messages: [...oldestFirst, ...state.messages],
        hasMoreHistory: older.length >= 50,
      );
    } catch (_) {
      // Silently keep existing state; the user can pull-to-refresh / retry scrolling.
    } finally {
      _isLoadingMore = false;
    }
  }

  void _subscribeToWsEvents() {
    _wsSubscription = _ws.events.listen((envelope) {
      switch (envelope.type) {
        case WsEventType.newMessage:
          _handleIncomingMessage(envelope.payload);
          break;
        case WsEventType.messageAck:
          _handleMessageAck(envelope.payload);
          break;
        case WsEventType.deliveryAck:
          _handleDeliveryAck(envelope.payload);
          break;
        case WsEventType.readAck:
          _handleReadAck(envelope.payload);
          break;
        case WsEventType.typingUpdate:
          _handleTypingUpdate(envelope.payload);
          break;
        case WsEventType.error:
          if (envelope.requestId != null) {
            _failPending(
              envelope.requestId!,
              envelope.payload['code'] == 'FORBIDDEN'
                  ? 'This conversation is unavailable'
                  : 'Message could not be sent',
            );
          }
          break;
        default:
          break;
      }
    });
  }

  void _handleConnectionStatus(WsConnectionStatus status) {
    if (!_initialized || !mounted) return;
    if (status == WsConnectionStatus.connected) {
      unawaited(_flushQueuedMessages());
      return;
    }
    if (status == WsConnectionStatus.disconnected) {
      final updated = <ChatMessage>[];
      var changed = false;
      for (final message in state.messages) {
        if (message.status == MessageDeliveryStatus.sending) {
          changed = true;
          final queued = message.copyWith(status: MessageDeliveryStatus.queued);
          updated.add(queued);
          final clientId = queued.clientGeneratedId;
          if (clientId != null) {
            _ackTimers.remove(clientId)?.cancel();
            unawaited(_persistBestEffort(queued));
          }
        } else {
          updated.add(message);
        }
      }
      if (changed) state = state.copyWith(messages: updated);
    }
  }

  Future<void> _flushQueuedMessages() async {
    if (!_allowPlaintextMessaging ||
        _flushingOutbox ||
        _ws.status != WsConnectionStatus.connected) {
      return;
    }
    _flushingOutbox = true;
    try {
      final queued = state.messages
          .where(
            (message) =>
                message.status == MessageDeliveryStatus.queued &&
                message.clientGeneratedId != null,
          )
          .toList();
      for (final message in queued) {
        if (!mounted || _ws.status != WsConnectionStatus.connected) break;
        _sendPending(message);
      }
    } finally {
      _flushingOutbox = false;
    }
  }

  void _handleIncomingMessage(Map<String, dynamic> payload) {
    if (payload['chatId'] != chatId) return;

    final incoming = ChatMessage.fromJson(
      payload,
      status: MessageDeliveryStatus.delivered,
    );
    final alreadyPresent = state.messages.any(
      (message) =>
          message.id == incoming.id ||
          (incoming.clientGeneratedId != null &&
              message.clientGeneratedId == incoming.clientGeneratedId),
    );
    if (!alreadyPresent) {
      state = state.copyWith(messages: [...state.messages, incoming]);
    }

    // Immediately ack delivery back to the sender, and mark it read since the chat
    // is presumed open/visible if this controller instance is alive and receiving.
    _ws.send(
      WsEnvelope(
        type: WsEventType.messageDelivered,
        payload: {'messageId': incoming.id},
      ),
    );
    _ws.send(
      WsEnvelope(
        type: WsEventType.messageRead,
        payload: {'messageId': incoming.id},
      ),
    );
  }

  void _handleMessageAck(Map<String, dynamic> payload) {
    final clientGeneratedId = payload['clientGeneratedId'] as String?;
    final messagePayload = payload['message'] as Map<String, dynamic>?;
    if (clientGeneratedId == null || messagePayload == null) return;
    if (messagePayload['chatId'] != chatId) return;

    final index = state.messages.indexWhere(
      (m) => m.clientGeneratedId == clientGeneratedId,
    );
    _ackTimers.remove(clientGeneratedId)?.cancel();
    unawaited(_removeBestEffort(clientGeneratedId));

    final serverMessage = ChatMessage.fromJson(
      messagePayload,
      status: MessageDeliveryStatus.sent,
    );
    if (index == -1) {
      state = state.copyWith(messages: [...state.messages, serverMessage]);
      return;
    }
    final updated = [...state.messages];
    final previousStatus = updated[index].status;
    updated[index] =
        _statusRank(previousStatus) > _statusRank(serverMessage.status)
        ? serverMessage.copyWith(status: previousStatus)
        : serverMessage;
    state = state.copyWith(messages: updated);
  }

  void _handleDeliveryAck(Map<String, dynamic> payload) {
    final messageId = payload['messageId'] as String?;
    if (messageId == null) return;
    _updateMessageStatus(messageId, MessageDeliveryStatus.delivered);
  }

  void _handleReadAck(Map<String, dynamic> payload) {
    final messageId = payload['messageId'] as String?;
    if (messageId == null) return;
    _updateMessageStatus(messageId, MessageDeliveryStatus.read);
  }

  void _updateMessageStatus(String messageId, MessageDeliveryStatus status) {
    final index = state.messages.indexWhere((m) => m.id == messageId);
    if (index == -1) return;

    // Never downgrade a status (e.g. a late delivery_ack arriving after read_ack
    // already processed) — read is a strictly "further along" state than delivered.
    final current = state.messages[index].status;
    if (_statusRank(status) <= _statusRank(current)) return;

    final updated = [...state.messages];
    updated[index] = updated[index].copyWith(status: status);
    state = state.copyWith(messages: updated);
  }

  int _statusRank(MessageDeliveryStatus s) {
    switch (s) {
      case MessageDeliveryStatus.queued:
        return 0;
      case MessageDeliveryStatus.sending:
        return 1;
      case MessageDeliveryStatus.sent:
        return 2;
      case MessageDeliveryStatus.delivered:
        return 3;
      case MessageDeliveryStatus.read:
        return 4;
      case MessageDeliveryStatus.failed:
        return -1;
    }
  }

  void _handleTypingUpdate(Map<String, dynamic> payload) {
    if (payload['chatId'] != chatId) return;
    final isTyping = payload['isTyping'] as bool? ?? false;
    state = state.copyWith(peerIsTyping: isTyping);
  }

  /// sendMessage queues an optimistic message locally, persists it in the encrypted
  /// outbox, then sends it over WS when connected. The server-assigned ID and
  /// createdAt replace this entry once message_ack arrives; clientGeneratedId stays
  /// the correlation and idempotency key through retries and process restarts.
  void sendMessage(String body) {
    final trimmed = body.trim();
    if (trimmed.isEmpty) return;
    if (!_allowPlaintextMessaging) {
      state = state.copyWith(
        errorMessage: 'Secure messaging is not available in this build',
      );
      return;
    }

    final clientGeneratedId = _uuid.v4();
    final optimistic = ChatMessage(
      id: clientGeneratedId, // temporary local ID until the real one arrives via ack
      chatId: chatId,
      senderId: currentUserId,
      type: 'text',
      body: trimmed,
      clientGeneratedId: clientGeneratedId,
      createdAt: DateTime.now(),
      status: MessageDeliveryStatus.queued,
    );

    state = state.copyWith(messages: [...state.messages, optimistic]);
    unawaited(_persistAndMaybeSend(optimistic));
  }

  Future<void> _persistAndMaybeSend(ChatMessage message) async {
    try {
      await _outbox.put(currentUserId, message);
    } catch (_) {
      _failPending(
        message.clientGeneratedId!,
        'Message could not be protected on this device',
        persist: false,
      );
      return;
    }
    if (mounted && _ws.status == WsConnectionStatus.connected) {
      _sendPending(message);
    }
  }

  void retryMessage(String id) {
    if (!_allowPlaintextMessaging) {
      return;
    }
    final index = state.messages.indexWhere(
      (m) => m.id == id && m.status == MessageDeliveryStatus.failed,
    );
    if (index < 0) return;
    final updated = [...state.messages];
    updated[index] = updated[index].copyWith(
      status: MessageDeliveryStatus.queued,
    );
    state = state.copyWith(messages: updated);
    unawaited(_persistAndMaybeSend(updated[index]));
  }

  void _failPending(String clientId, String error, {bool persist = true}) {
    if (!mounted) return;
    _ackTimers.remove(clientId)?.cancel();
    final index = state.messages.indexWhere(
      (m) =>
          m.clientGeneratedId == clientId &&
          (m.status == MessageDeliveryStatus.queued ||
              m.status == MessageDeliveryStatus.sending),
    );
    if (index < 0) return;
    final updated = [...state.messages];
    updated[index] = updated[index].copyWith(
      status: MessageDeliveryStatus.failed,
    );
    state = state.copyWith(messages: updated, errorMessage: error);
    if (persist) unawaited(_persistBestEffort(updated[index]));
  }

  Future<void> _persistBestEffort(ChatMessage message) async {
    try {
      await _outbox.put(currentUserId, message);
    } catch (_) {
      // The message is already visible as queued/failed. A later explicit retry
      // will attempt durable storage again before anything leaves the device.
    }
  }

  Future<void> _removeBestEffort(String clientGeneratedId) async {
    try {
      await _outbox.remove(clientGeneratedId);
    } catch (_) {
      // A stale idempotent outbox record is reconciled against REST history on
      // the next controller start and never creates a duplicate server message.
    }
  }

  void _sendPending(ChatMessage message) {
    if (!_allowPlaintextMessaging ||
        !mounted ||
        _ws.status != WsConnectionStatus.connected) {
      return;
    }
    final clientGeneratedId = message.clientGeneratedId!;
    final index = state.messages.indexWhere(
      (candidate) => candidate.clientGeneratedId == clientGeneratedId,
    );
    if (index >= 0) {
      final updated = [...state.messages];
      updated[index] = updated[index].copyWith(
        status: MessageDeliveryStatus.sending,
      );
      state = state.copyWith(messages: updated);
    }
    _ackTimers.remove(clientGeneratedId)?.cancel();
    _ackTimers[clientGeneratedId] = Timer(
      const Duration(seconds: 20),
      () => _failPending(
        clientGeneratedId,
        'Message confirmation timed out. You can retry.',
      ),
    );

    _ws.send(
      WsEnvelope(
        type: WsEventType.sendMessage,
        requestId: clientGeneratedId,
        payload: {
          'chatId': chatId,
          'type': 'text',
          'body': message.body,
          'clientGeneratedId': clientGeneratedId,
        },
      ),
    );
  }

  void notifyTypingStart() {
    if (!_allowPlaintextMessaging) return;
    _ws.send(
      WsEnvelope(type: WsEventType.typingStart, payload: {'chatId': chatId}),
    );

    _typingStopTimer?.cancel();
    _typingStopTimer = Timer(const Duration(seconds: 3), notifyTypingStop);
  }

  void notifyTypingStop() {
    if (!_allowPlaintextMessaging) return;
    _typingStopTimer?.cancel();
    _ws.send(
      WsEnvelope(type: WsEventType.typingStop, payload: {'chatId': chatId}),
    );
  }

  @override
  void dispose() {
    _wsSubscription?.cancel();
    _statusSubscription?.cancel();
    _typingStopTimer?.cancel();
    for (final timer in _ackTimers.values) {
      timer.cancel();
    }
    _ackTimers.clear();
    super.dispose();
  }
}

/// chatControllerProvider is a `.family` keyed by chatId, so navigating between
/// different chats gets independent controller instances — each disposed when its
/// screen is popped, rather than one giant shared controller for every open chat.
final chatControllerProvider = StateNotifierProvider.autoDispose
    .family<ChatController, ChatState, String>((ref, chatId) {
      final userId =
          ref.watch(authControllerProvider.select((s) => s.userId)) ?? '';
      return ChatController(
        chatId: chatId,
        currentUserId: userId,
        remote: ref.read(messageRemoteDataSourceProvider),
        ws: ref.read(webSocketClientProvider),
        outbox: ref.read(messageOutboxStoreProvider),
        allowPlaintextMessaging: Env.allowPlaintextMessaging,
      );
    });
