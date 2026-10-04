import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:bharatchat/core/network/websocket_client.dart';

void main() {
  test('disconnect cancels an in-flight connection ticket', () async {
    final ticket = Completer<String?>();
    final client = WebSocketClientService(ticketProvider: () => ticket.future);
    final connecting = client.connect();
    client.disconnect();
    ticket.complete('stale-ticket');
    await connecting;
    expect(client.status, WsConnectionStatus.disconnected);
    client.dispose();
  });
  test(
    'dispose while a ticket is pending does not revive a socket or throw',
    () async {
      final ticket = Completer<String?>();
      final client = WebSocketClientService(
        ticketProvider: () => ticket.future,
      );
      final connecting = client.connect();
      client.dispose();
      ticket.complete('stale-ticket');
      await connecting;
      await client.connect();
      expect(client.status, WsConnectionStatus.disconnected);
    },
  );
}
