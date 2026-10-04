import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../auth/presentation/providers/auth_providers.dart';
import '../../../chat_list/presentation/providers/chat_list_providers.dart';
import '../../domain/entities/group_info.dart';
import '../providers/group_providers.dart';

class GroupInfoScreen extends ConsumerWidget {
  final String chatId;
  const GroupInfoScreen({super.key, required this.chatId});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(groupControllerProvider(chatId));
    final controller = ref.read(groupControllerProvider(chatId).notifier);
    final current = ref.watch(authControllerProvider).userId;
    if (state.loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (state.info == null) {
      return Scaffold(
        appBar: AppBar(),
        body: Center(child: Text(state.error ?? 'Group not found')),
      );
    }
    final info = state.info!;
    final me = state.members.where((m) => m.userId == current).firstOrNull;
    final admin = me?.role == 'owner' || me?.role == 'admin';
    return Scaffold(
      appBar: AppBar(
        title: const Text('Group info'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: controller.refresh,
            icon: const Icon(Icons.refresh),
          ),
        ],
      ),
      body: ListView(
        children: [
          Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              children: [
                CircleAvatar(
                  radius: 42,
                  backgroundImage: info.iconUrl == null
                      ? null
                      : NetworkImage(info.iconUrl!),
                  child: info.iconUrl == null
                      ? const Icon(Icons.group, size: 42)
                      : null,
                ),
                const SizedBox(height: 12),
                Text(
                  info.name,
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                if (info.description?.isNotEmpty == true)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(info.description!, textAlign: TextAlign.center),
                  ),
              ],
            ),
          ),
          const Divider(height: 1),
          if (admin) ...[
            SwitchListTile(
              title: const Text('Only admins can post'),
              value: info.onlyAdminsCanPost,
              onChanged: state.saving
                  ? null
                  : (v) => controller.action(
                      () => controller.remote.setAdminsPost(chatId, v),
                    ),
            ),
            SwitchListTile(
              title: const Text('Only admins can edit info'),
              value: info.onlyAdminsCanEditInfo,
              onChanged: state.saving
                  ? null
                  : (v) => controller.action(
                      () => controller.remote.setAdminsEdit(chatId, v),
                    ),
            ),
          ],
          ListTile(
            title: const Text('Invite code'),
            subtitle: Text(
              info.inviteCodeEnabled
                  ? (info.inviteCode ?? 'Not generated')
                  : 'Disabled',
            ),
            trailing: admin
                ? PopupMenuButton<String>(
                    onSelected: (v) {
                      if (v == 'copy' && info.inviteCode != null) {
                        Clipboard.setData(
                          ClipboardData(text: info.inviteCode!),
                        );
                      }
                      if (v == 'regen') {
                        controller.action(
                          () => controller.remote
                              .regenerateCode(chatId)
                              .then((_) {}),
                        );
                      }
                      if (v == 'toggle') {
                        controller.action(
                          () => controller.remote.setInviteEnabled(
                            chatId,
                            !info.inviteCodeEnabled,
                          ),
                        );
                      }
                    },
                    itemBuilder: (_) => [
                      if (info.inviteCode != null)
                        const PopupMenuItem(value: 'copy', child: Text('Copy')),
                      const PopupMenuItem(
                        value: 'regen',
                        child: Text('Regenerate'),
                      ),
                      PopupMenuItem(
                        value: 'toggle',
                        child: Text(
                          info.inviteCodeEnabled ? 'Disable' : 'Enable',
                        ),
                      ),
                    ],
                  )
                : null,
          ),
          const Divider(height: 1),
          ListTile(
            title: Text('${state.members.length} members'),
            trailing: admin
                ? IconButton(
                    tooltip: 'Add member',
                    onPressed: () => _add(context, controller),
                    icon: const Icon(Icons.person_add_outlined),
                  )
                : null,
          ),
          for (final member in state.members)
            _MemberTile(
              member: member,
              currentUserId: current ?? '',
              myRole: me?.role ?? '',
              onAction: (action) =>
                  _memberAction(context, ref, controller, member, action),
            ),
          const Divider(height: 1),
          ListTile(
            textColor: Theme.of(context).colorScheme.error,
            iconColor: Theme.of(context).colorScheme.error,
            leading: const Icon(Icons.exit_to_app),
            title: const Text('Leave group'),
            onTap: () => _leave(context, ref, controller),
          ),
        ],
      ),
    );
  }

  Future<void> _add(BuildContext context, GroupController c) async {
    final id = await _ask(context, 'Add member', 'User ID');
    if (id != null) await c.action(() => c.remote.addMember(chatId, id));
  }

  Future<void> _memberAction(
    BuildContext context,
    WidgetRef ref,
    GroupController c,
    GroupMember member,
    String action,
  ) async {
    switch (action) {
      case 'remove':
        await c.action(() => c.remote.removeMember(chatId, member.userId));
        return;
      case 'admin':
        await c.action(
          () =>
              c.remote.setAdmin(chatId, member.userId, member.role != 'admin'),
        );
        return;
      case 'owner':
        final ok = await _confirm(
          context,
          'Transfer ownership to ${member.displayName}?',
        );
        if (ok) {
          await c.action(() => c.remote.transfer(chatId, member.userId));
        }
        return;
    }
  }

  Future<void> _leave(
    BuildContext context,
    WidgetRef ref,
    GroupController c,
  ) async {
    if (!await _confirm(context, 'Leave this group?')) return;
    try {
      await c.remote.leave(chatId);
      await ref.read(chatListProvider.notifier).refresh();
      if (context.mounted) context.go('/home');
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Transfer ownership before leaving')),
        );
      }
    }
  }

  Future<String?> _ask(BuildContext context, String title, String label) {
    final input = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(title),
        content: TextField(
          controller: input,
          decoration: InputDecoration(labelText: label),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, input.text.trim()),
            child: const Text('Add'),
          ),
        ],
      ),
    ).whenComplete(input.dispose);
  }

  Future<bool> _confirm(BuildContext context, String text) async =>
      await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          content: Text(text),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Continue'),
            ),
          ],
        ),
      ) ??
      false;
}

class _MemberTile extends StatelessWidget {
  final GroupMember member;
  final String currentUserId, myRole;
  final ValueChanged<String> onAction;
  const _MemberTile({
    required this.member,
    required this.currentUserId,
    required this.myRole,
    required this.onAction,
  });
  @override
  Widget build(BuildContext context) {
    final owner = myRole == 'owner';
    final admin = owner || myRole == 'admin';
    final canRemove =
        admin && member.role != 'owner' && member.userId != currentUserId;
    return ListTile(
      leading: CircleAvatar(
        backgroundImage: member.avatarUrl == null
            ? null
            : NetworkImage(member.avatarUrl!),
        child: member.avatarUrl == null
            ? Text(
                member.displayName.isEmpty
                    ? '?'
                    : member.displayName[0].toUpperCase(),
              )
            : null,
      ),
      title: Text(member.displayName),
      subtitle: Text(member.role),
      trailing: !canRemove && !owner
          ? null
          : PopupMenuButton<String>(
              onSelected: onAction,
              itemBuilder: (_) => [
                if (owner &&
                    member.role != 'owner' &&
                    member.userId != currentUserId)
                  PopupMenuItem(
                    value: 'admin',
                    child: Text(
                      member.role == 'admin' ? 'Remove admin' : 'Make admin',
                    ),
                  ),
                if (owner &&
                    member.role != 'owner' &&
                    member.userId != currentUserId)
                  const PopupMenuItem(
                    value: 'owner',
                    child: Text('Transfer ownership'),
                  ),
                if (canRemove)
                  const PopupMenuItem(value: 'remove', child: Text('Remove')),
              ],
            ),
    );
  }
}

extension _FirstOrNull<E> on Iterable<E> {
  E? get firstOrNull {
    final i = iterator;
    return i.moveNext() ? i.current : null;
  }
}
