import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../chat_list/presentation/providers/chat_list_providers.dart';
import '../providers/group_providers.dart';

class JoinGroupScreen extends ConsumerStatefulWidget {
  final String initialCode;

  const JoinGroupScreen({super.key, this.initialCode = ''});

  @override
  ConsumerState<JoinGroupScreen> createState() => _JoinGroupScreenState();
}

class _JoinGroupScreenState extends ConsumerState<JoinGroupScreen> {
  late final TextEditingController _code = TextEditingController(
    text: widget.initialCode,
  );
  bool _loading = false;

  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  Future<void> _join() async {
    if (_code.text.trim().isEmpty) return;
    setState(() => _loading = true);

    try {
      final group = await ref
          .read(groupRemoteDataSourceProvider)
          .join(_code.text.trim());
      await ref.read(chatListProvider.notifier).refresh();
      if (mounted) {
        context.go(
          '/chat/${group.chatId}?title=${Uri.encodeQueryComponent(group.name)}&group=true',
        );
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Invite code is invalid or unavailable'),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Join group')),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 440),
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextField(
                  controller: _code,
                  textCapitalization: TextCapitalization.characters,
                  decoration: const InputDecoration(labelText: 'Invite code'),
                ),
                const SizedBox(height: 16),
                FilledButton(
                  onPressed: _loading ? null : _join,
                  child: const Text('Join'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
