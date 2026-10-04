import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;

import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:web_socket_channel/status.dart' as ws_status;

import '../config/env.dart';

typedef WebSocketTicketProvider = Future<String?> Function();

/// The closed set of event types this app sends/receives — mirrors the Go
/// backend's websocket.EventType enum exactly (internal/platform/websocket/message.go).
/// Keeping this as an enum (not free strings) is what lets the compiler catch a
/// typo on either side of a future protocol change.
enum WsEventType {
  sendMessage,
  messageDelivered,
  messageRead,
  typingStart,
  typingStop,
  newMessage,
  messageAck,
  deliveryAck,
  readAck,
  typingUpdate,
  presenceUpdate,
  error,
}

const _wsEventTypeWire = {
  WsEventType.sendMessage: 'send_message',
  WsEventType.messageDelivered: 'message_delivered',
  WsEventType.messageRead: 'message_read',
  WsEventType.typingStart: 'typing_start',
  WsEventType.typingStop: 'typing_stop',
  WsEventType.newMessage: 'new_message',
  WsEventType.messageAck: 'message_ack',
  WsEventType.deliveryAck: 'delivery_ack',
  WsEventType.readAck: 'read_ack',
  WsEventType.typingUpdate: 'typing_update',
  WsEventType.presenceUpdate: 'presence_update',
  WsEventType.error: 'error',
};

WsEventType? _wsEventTypeFromWire(String wire) {
  for (final entry in _wsEventTypeWire.entries) {
    if (entry.value == wire) return entry.key;
  }
  return null;
}

class WsEnvelope {
  final WsEventType type;
  final Map<String, dynamic> payload;
  final String? requestId;

  WsEnvelope({required this.type, required this.payload, this.requestId});

  factory WsEnvelope.fromJson(Map<String, dynamic> json) {
    final type = _wsEventTypeFromWire(json['type'] as String);
    if (type == null) {
      throw FormatException('Unknown WS event type: ${json['type']}');
    }
    return WsEnvelope(
      type: type,
      payload: (json['payload'] as Map<String, dynamic>?) ?? {},
      requestId: json['requestId'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
    'type': _wsEventTypeWire[type],
    'payload': payload,
    if (requestId != null) 'requestId': requestId,
  };
}

enum WsConnectionStatus { disconnected, connecting, connected }

/// A self-reconnecting WebSocket client with exponential backoff. Every feature
/// that needs real-time updates (chat_list for unread counts / online status,
/// chat for messages/typing/acks) listens to the same [events] broadcast stream
/// rather than each feature opening its own connection — one socket per app
/// session, fanned out in-process via a broadcast StreamController.
class WebSocketClientService {
  final WebSocketTicketProvider _ticketProvider;

  WebSocketChannel? _channel;
  StreamSubscription? _channelSubscription;
  Timer? _reconnectTimer;
  int _reconnectAttempt = 0;
  bool _manuallyClosed = false;
  bool _disposed = false;
  int _generation = 0;

  final _eventsController = StreamController<WsEnvelope>.broadcast();
  final _statusController = StreamController<WsConnectionStatus>.broadcast();

  WsConnectionStatus _status = WsConnectionStatus.disconnected;

  WebSocketClientService({required WebSocketTicketProvider ticketProvider})
    : _ticketProvider = ticketProvider;

  Stream<WsEnvelope> get events => _eventsController.stream;
  Stream<WsConnectionStatus> get statusStream => _statusController.stream;
  WsConnectionStatus get status => _status;

  Future<void> connect() async {
    if (_disposed) return;
    _manuallyClosed = false;
    await _connectInternal();
  }

  Future<void> _connectInternal() async {
    if (_disposed || _manuallyClosed) return;
    if (_status == WsConnectionStatus.connecting ||
        _status == WsConnectionStatus.connected) {
      return;
    }
    _setStatus(WsConnectionStatus.connecting);
    final generation = ++_generation;
    String? ticket;
    try {
      ticket = await _ticketProvider();
    } catch (_) {
      if (!_disposed && generation == _generation) _handleDisconnect();
      return;
    }
    if (_disposed || _manuallyClosed || generation != _generation) return;
    if (ticket == null || ticket.isEmpty) {
      _setStatus(WsConnectionStatus.disconnected);
      if (!_manuallyClosed) _scheduleReconnect();
      return;
    }

    final baseUri = Uri.parse(
      '${Env.baseUrl.replaceFirst(RegExp(r'^http'), 'ws')}/ws',
    );
    try {
      final channel = WebSocketChannel.connect(
        baseUri,
        protocols: ['bharatchat.v1', 'ticket.$ticket'],
        // Browsers cannot attach an Authorization header to WebSocket handshakes.
        // A 30-second, single-use ticket travels as a WebSocket subprotocol so
        // neither it nor a JWT appears in browser history or proxy URL logs.
      );
      await channel.ready;
      if (_disposed || _manuallyClosed || generation != _generation) {
        await channel.sink.close(ws_status.normalClosure);
        return;
      }

      _channel = channel;
      _reconnectAttempt = 0;
      _setStatus(WsConnectionStatus.connected);

      _channelSubscription = channel.stream.listen(
        _handleRawMessage,
        onDone: () {
          if (generation == _generation) _handleDisconnect();
        },
        onError: (_) {
          if (generation == _generation) _handleDisconnect();
        },
        cancelOnError: true,
      );
    } catch (_) {
      if (!_disposed && generation == _generation) _handleDisconnect();
    }
  }

  void _handleRawMessage(dynamic raw) {
    try {
      final decoded = jsonDecode(raw as String) as Map<String, dynamic>;
      final envelope = WsEnvelope.fromJson(decoded);
      _eventsController.add(envelope);
    } catch (_) {
      // Malformed frame: ignore rather than crash the whole connection handling —
      // a single bad frame must never take down the socket for every other event.
    }
  }

  void _handleDisconnect() {
    if (_disposed) return;
    _channelSubscription?.cancel();
    _channel = null;
    _setStatus(WsConnectionStatus.disconnected);

    if (!_manuallyClosed) {
      _scheduleReconnect();
    }
  }

  void _scheduleReconnect() {
    _reconnectTimer?.cancel();
    _reconnectAttempt++;

    // Exponential backoff capped at 30s: 1s, 2s, 4s, 8s, 16s, 30s, 30s, ...
    final exponent = math.min(math.max(_reconnectAttempt - 1, 0), 5);
    final delaySeconds = math.min(1 << exponent, 30);
    _reconnectTimer = Timer(Duration(seconds: delaySeconds), () {
      if (!_manuallyClosed) {
        _connectInternal();
      }
    });
  }

  void send(WsEnvelope envelope) {
    final channel = _channel;
    if (channel == null || _status != WsConnectionStatus.connected) {
      // Fire-and-forget by design at this layer: callers (e.g. ChatController)
      // are responsible for optimistic UI + eventual consistency via REST history
      // if a send is attempted while disconnected — the socket layer itself does
      // not queue offline sends, keeping this class's responsibility narrow.
      return;
    }
    channel.sink.add(jsonEncode(envelope.toJson()));
  }

  void disconnect() {
    _generation++;
    _manuallyClosed = true;
    _reconnectTimer?.cancel();
    _channelSubscription?.cancel();
    _channel?.sink.close(ws_status.normalClosure);
    _channel = null;
    _setStatus(WsConnectionStatus.disconnected);
  }

  void _setStatus(WsConnectionStatus newStatus) {
    if (_disposed) return;
    _status = newStatus;
    _statusController.add(newStatus);
  }

  void dispose() {
    if (_disposed) return;
    disconnect();
    _disposed = true;
    _eventsController.close();
    _statusController.close();
  }
}
