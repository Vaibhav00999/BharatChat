import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../auth/presentation/providers/auth_providers.dart';
import '../../domain/entities/chat_summary.dart';
import '../providers/chat_list_providers.dart';

class ChatListScreen extends ConsumerWidget {
  const ChatListScreen({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final chats = ref.watch(chatListProvider);
    return Scaffold(
      appBar: AppBar(
        title: const Text('BharatChat'),
        actions: [
          IconButton(
            tooltip: 'Privacy',
            onPressed: () => context.push('/privacy'),
            icon: const Icon(Icons.shield_outlined),
          ),
          IconButton(
            tooltip: 'Create group',
            onPressed: () => context.push('/groups/create'),
            icon: const Icon(Icons.group_add_outlined),
          ),
          PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'join') _join(context);
              if (v == 'logout') {
                ref.read(authControllerProvider.notifier).logout();
              }
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'join', child: Text('Join group')),
              PopupMenuItem(value: 'logout', child: Text('Log out')),
            ],
          ),
        ],
      ),
      body: chats.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => _ErrorView(
          onRetry: () => ref.read(chatListProvider.notifier).refresh(),
        ),
        data: (items) => items.isEmpty
            ? const Center(child: Text('No conversations yet'))
            : RefreshIndicator(
                onRefresh: () => ref.read(chatListProvider.notifier).refresh(),
                child: ListView.separated(
                  itemCount: items.length,
                  separatorBuilder: (_, __) => const Divider(height: 1),
                  itemBuilder: (context, i) => _ChatTile(chat: items[i]),
                ),
              ),
      ),
      floatingActionButton: FloatingActionButton(
        tooltip: 'New chat',
        onPressed: () => context.push('/chats/new'),
        child: const Icon(Icons.chat_outlined),
      ),
    );
  }

  Future<void> _join(BuildContext context) async {
    final code = await _ask(context, 'Join a group', 'Invite code');
    if (code != null && context.mounted) {
      context.push('/groups/join?code=${Uri.encodeQueryComponent(code)}');
    }
  }

  Future<String?> _ask(BuildContext context, String title, String label) {
    final c = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(title),
        content: TextField(
          controller: c,
          decoration: InputDecoration(labelText: label),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, c.text.trim()),
            child: const Text('Continue'),
          ),
        ],
      ),
    ).whenComplete(c.dispose);
  }
}

class _ErrorView extends StatelessWidget {
  final VoidCallback onRetry;
  const _ErrorView({required this.onRetry});
  @override
  Widget build(BuildContext context) => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Text('Could not load chats'),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: onRetry,
          icon: const Icon(Icons.refresh),
          label: const Text('Retry'),
        ),
      ],
    ),
  );
}

class _ChatTile extends StatelessWidget {
  final ChatSummary chat;
  const _ChatTile({required this.chat});
  @override
  Widget build(BuildContext context) {
    final name = chat.displayName;
    var preview = chat.lastMessageBody ?? 'No messages yet';
    if (chat.type == 'group' &&
        chat.lastMessageSenderName != null &&
        chat.lastMessageBody != null) {
      preview = '${chat.lastMessageSenderName}: ${chat.lastMessageBody}';
    }
    return ListTile(
      onTap: () => context.push(
        '/chat/${chat.id}?title=${Uri.encodeQueryComponent(name)}&group=${chat.type == 'group'}&peer=${chat.peerUserId ?? ''}',
      ),
      leading: Stack(
        children: [
          CircleAvatar(
            backgroundImage: chat.displayAvatar == null
                ? null
                : NetworkImage(chat.displayAvatar!),
            child: chat.displayAvatar == null
                ? Icon(chat.type == 'group' ? Icons.group : Icons.person)
                : null,
          ),
          if (chat.type == 'direct' && chat.peerIsOnline)
            Positioned(
              right: 0,
              bottom: 0,
              child: Container(
                width: 11,
                height: 11,
                decoration: BoxDecoration(
                  color: Colors.green,
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: Theme.of(context).scaffoldBackgroundColor,
                    width: 2,
                  ),
                ),
              ),
            ),
        ],
      ),
      title: Text(
        name,
        style: TextStyle(
          fontWeight: chat.unreadCount > 0 ? FontWeight.w700 : FontWeight.w500,
        ),
      ),
      subtitle: Text(preview, maxLines: 1, overflow: TextOverflow.ellipsis),
      trailing: chat.unreadCount == 0
          ? null
          : Badge(
              label: Text(
                chat.unreadCount > 99 ? '99+' : '${chat.unreadCount}',
              ),
            ),
    );
  }
}
