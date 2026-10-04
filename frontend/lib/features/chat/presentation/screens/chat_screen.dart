// lib/features/chat/presentation/screens/chat_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../auth/presentation/providers/auth_providers.dart';
import '../../../profile/presentation/screens/people_screens.dart';

import '../../domain/entities/chat_message.dart';
import '../providers/chat_providers.dart';

class ChatScreen extends ConsumerStatefulWidget {
  final String chatId;
  final String title;
  final bool isGroup;
  final String? peerUserId;
  const ChatScreen({
    super.key,
    required this.chatId,
    this.title = 'Chat',
    this.isGroup = false,
    this.peerUserId,
  });

  @override
  ConsumerState<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends ConsumerState<ChatScreen> {
  final _textController = TextEditingController();
  final _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
  }

  void _onScroll() {
    if (_scrollController.position.pixels <= 100) {
      ref
          .read(chatControllerProvider(widget.chatId).notifier)
          .loadMoreHistory();
    }
  }

  @override
  void dispose() {
    _textController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  void _send() {
    final text = _textController.text;
    if (text.trim().isEmpty) return;
    ref.read(chatControllerProvider(widget.chatId).notifier).sendMessage(text);
    _textController.clear();
    ref.read(chatControllerProvider(widget.chatId).notifier).notifyTypingStop();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(chatControllerProvider(widget.chatId));
    final currentUserId = ref.watch(
      authControllerProvider.select((s) => s.userId),
    );

    return Scaffold(
      appBar: AppBar(
        title: Text(state.peerIsTyping ? 'typing...' : widget.title),
        actions: [
          if (!widget.isGroup &&
              widget.peerUserId != null &&
              widget.peerUserId!.isNotEmpty)
            PopupMenuButton<String>(
              tooltip: 'Chat safety',
              onSelected: (value) {
                if (value == 'block') {
                  blockPerson(context, ref, widget.peerUserId!);
                }
                if (value == 'report') {
                  reportPerson(context, widget.peerUserId!);
                }
              },
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'block', child: Text('Block user')),
                PopupMenuItem(value: 'report', child: Text('Report user')),
              ],
            ),
          if (widget.isGroup)
            IconButton(
              tooltip: 'Group info',
              onPressed: () => context.push('/groups/${widget.chatId}'),
              icon: const Icon(Icons.info_outline),
            ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: state.isLoadingHistory
                ? const Center(child: CircularProgressIndicator())
                : ListView.builder(
                    controller: _scrollController,
                    itemCount: state.messages.length,
                    itemBuilder: (context, index) {
                      final message = state.messages[index];
                      final isMine = message.senderId == currentUserId;
                      return _MessageBubble(
                        message: message,
                        isMine: isMine,
                        onRetry: () => ref
                            .read(
                              chatControllerProvider(widget.chatId).notifier,
                            )
                            .retryMessage(message.id),
                      );
                    },
                  ),
          ),
          if (state.errorMessage != null)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              child: Text(
                state.errorMessage!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
                textAlign: TextAlign.center,
              ),
            ),
          _MessageComposer(
            controller: _textController,
            onSend: _send,
            enabled: state.canSendMessages,
            onChanged: (text) {
              if (text.isEmpty) {
                ref
                    .read(chatControllerProvider(widget.chatId).notifier)
                    .notifyTypingStop();
              } else {
                ref
                    .read(chatControllerProvider(widget.chatId).notifier)
                    .notifyTypingStart();
              }
            },
          ),
        ],
      ),
    );
  }
}

class _MessageBubble extends StatelessWidget {
  final ChatMessage message;
  final bool isMine;
  final VoidCallback onRetry;
  const _MessageBubble({
    required this.message,
    required this.isMine,
    required this.onRetry,
  });

  Widget _statusIcon(BuildContext context) {
    switch (message.status) {
      case MessageDeliveryStatus.queued:
        return const Icon(
          Icons.cloud_upload_outlined,
          size: 14,
          color: Colors.grey,
        );
      case MessageDeliveryStatus.sending:
        return const Icon(Icons.access_time, size: 14, color: Colors.grey);
      case MessageDeliveryStatus.sent:
        return const Icon(Icons.check, size: 14, color: Colors.grey);
      case MessageDeliveryStatus.delivered:
        return const Icon(Icons.done_all, size: 14, color: Colors.grey);
      case MessageDeliveryStatus.read:
        return Icon(
          Icons.done_all,
          size: 14,
          color: Theme.of(context).colorScheme.primary,
        );
      case MessageDeliveryStatus.failed:
        return const Icon(Icons.error_outline, size: 14, color: Colors.red);
    }
  }

  @override
  Widget build(BuildContext context) {
    final bubbleColor = isMine
        ? Theme.of(context).colorScheme.primaryContainer
        : Theme.of(context).colorScheme.surfaceContainerHighest;

    return Align(
      alignment: isMine ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        constraints: BoxConstraints(
          maxWidth: MediaQuery.of(context).size.width * 0.75,
        ),
        decoration: BoxDecoration(
          color: bubbleColor,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.end,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(message.body ?? ''),
            if (isMine && message.status == MessageDeliveryStatus.failed)
              IconButton(
                tooltip: 'Retry message',
                onPressed: onRetry,
                icon: const Icon(Icons.refresh),
              ),
            const SizedBox(height: 4),
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  '${message.createdAt.hour.toString().padLeft(2, '0')}:${message.createdAt.minute.toString().padLeft(2, '0')}',
                  style: Theme.of(context).textTheme.labelSmall,
                ),
                if (isMine) ...[const SizedBox(width: 4), _statusIcon(context)],
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _MessageComposer extends StatelessWidget {
  final TextEditingController controller;
  final VoidCallback onSend;
  final ValueChanged<String> onChanged;
  final bool enabled;

  const _MessageComposer({
    required this.controller,
    required this.onSend,
    required this.onChanged,
    required this.enabled,
  });

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
        child: Row(
          children: [
            Expanded(
              child: TextField(
                enabled: enabled,
                controller: controller,
                onChanged: onChanged,
                decoration: InputDecoration(
                  hintText: enabled
                      ? 'Message'
                      : 'Secure messaging unavailable',
                  border: const OutlineInputBorder(
                    borderRadius: BorderRadius.all(Radius.circular(24)),
                  ),
                  contentPadding: const EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 10,
                  ),
                ),
                textInputAction: TextInputAction.send,
                onSubmitted: (_) => onSend(),
                minLines: 1,
                maxLines: 5,
              ),
            ),
            const SizedBox(width: 8),
            IconButton.filled(
              tooltip: 'Send',
              onPressed: enabled ? onSend : null,
              icon: const Icon(Icons.send),
            ),
          ],
        ),
      ),
    );
  }
}
