import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../auth/presentation/providers/auth_providers.dart';
import '../../../chat_list/presentation/providers/chat_list_providers.dart';
import '../../data/people_remote_datasource.dart';

final peopleRemoteProvider = Provider(
  (ref) => PeopleRemoteDataSource(ref.read(dioProvider)),
);

class NewChatScreen extends ConsumerStatefulWidget {
  const NewChatScreen({super.key});
  @override
  ConsumerState<NewChatScreen> createState() => _NewChatScreenState();
}

class _NewChatScreenState extends ConsumerState<NewChatScreen> {
  final _username = TextEditingController();
  Person? _person;
  String? _error;
  bool _busy = false;
  @override
  void dispose() {
    _username.dispose();
    super.dispose();
  }

  Future<void> _find() async {
    final name = _username.text
        .trim()
        .replaceFirst(RegExp(r'^@'), '')
        .toLowerCase();
    if (!RegExp(r'^[a-z0-9]{3,30}$').hasMatch(name)) {
      setState(() {
        _error = 'Enter a username with 3-30 letters or digits';
        _person = null;
      });
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
      _person = null;
    });
    try {
      final person = await ref.read(peopleRemoteProvider).resolve(name);
      if (mounted) setState(() => _person = person);
    } catch (e) {
      if (mounted) setState(() => _error = peopleError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _start() async {
    final person = _person;
    if (person == null || _busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final id = await ref
          .read(chatListProvider.notifier)
          .startDirectChat(person.id);
      if (mounted) {
        context.pushReplacement(
          Uri(
            path: '/chat/$id',
            queryParameters: {'title': person.displayName, 'peer': person.id},
          ).toString(),
        );
      }
    } catch (e) {
      if (mounted) setState(() => _error = peopleError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('New chat')),
    body: Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 600),
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            TextField(
              controller: _username,
              enabled: !_busy,
              autocorrect: false,
              textInputAction: TextInputAction.search,
              onSubmitted: (_) => _find(),
              onChanged: (_) => setState(() {
                _person = null;
                _error = null;
              }),
              decoration: InputDecoration(
                labelText: 'Exact username',
                prefixText: '@',
                border: const OutlineInputBorder(),
                suffixIcon: IconButton(
                  tooltip: 'Find user',
                  onPressed: _busy ? null : _find,
                  icon: const Icon(Icons.search),
                ),
              ),
            ),
            if (_busy)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 16),
                child: LinearProgressIndicator(),
              ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            if (_person != null) ...[
              const SizedBox(height: 16),
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const CircleAvatar(child: Icon(Icons.person_outline)),
                title: Text(_person!.displayName),
                subtitle: Text('@${_person!.username}'),
                trailing: IconButton(
                  tooltip: 'Start chat',
                  onPressed: _busy ? null : _start,
                  icon: const Icon(Icons.chat_outlined),
                ),
              ),
            ],
          ],
        ),
      ),
    ),
  );
}

class BlockedUsersScreen extends ConsumerStatefulWidget {
  const BlockedUsersScreen({super.key});
  @override
  ConsumerState<BlockedUsersScreen> createState() => _BlockedUsersScreenState();
}

class _BlockedUsersScreenState extends ConsumerState<BlockedUsersScreen> {
  List<Person> _users = [];
  String? _next;
  String? _error;
  bool _busy = false;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool more = false}) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final page = await ref
          .read(peopleRemoteProvider)
          .blocked(after: more ? _next : null);
      if (mounted) {
        setState(() {
          _users = more ? [..._users, ...page.users] : page.users;
          _next = page.nextCursor;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = peopleError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _unblock(Person person) async {
    final approved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Unblock ${person.displayName}?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Unblock'),
          ),
        ],
      ),
    );
    if (approved != true || !mounted) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref.read(peopleRemoteProvider).unblock(person.id);
      if (mounted) setState(() => _users.removeWhere((p) => p.id == person.id));
    } catch (e) {
      if (mounted) setState(() => _error = peopleError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Blocked users'),
      actions: [
        IconButton(
          tooltip: 'Refresh',
          onPressed: _busy ? null : _load,
          icon: const Icon(Icons.refresh),
        ),
      ],
    ),
    body: ListView(
      padding: const EdgeInsets.symmetric(vertical: 12),
      children: [
        if (_busy) const LinearProgressIndicator(),
        if (_error != null)
          Padding(padding: const EdgeInsets.all(16), child: Text(_error!)),
        if (!_busy && _users.isEmpty && _error == null)
          const Padding(
            padding: EdgeInsets.all(24),
            child: Text('No blocked users'),
          ),
        for (final person in _users)
          ListTile(
            leading: const Icon(Icons.block),
            title: Text(person.displayName),
            subtitle: person.username == null
                ? null
                : Text('@${person.username}'),
            trailing: IconButton(
              tooltip: 'Unblock',
              onPressed: _busy ? null : () => _unblock(person),
              icon: const Icon(Icons.person_add_alt_1),
            ),
          ),
        if (_next != null)
          Center(
            child: TextButton(
              onPressed: _busy ? null : () => _load(more: true),
              child: const Text('Load more'),
            ),
          ),
      ],
    ),
  );
}

Future<void> blockPerson(BuildContext context, WidgetRef ref, String id) async {
  final approved = await showDialog<bool>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('Block this user?'),
      content: const Text(
        'Direct messages and typing updates will stop. Shared groups are unaffected.',
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context, false),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context, true),
          child: const Text('Block'),
        ),
      ],
    ),
  );
  if (approved != true || !context.mounted) return;
  try {
    await ref.read(peopleRemoteProvider).block(id);
    if (context.mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('User blocked')));
      context.go('/home');
    }
  } catch (e) {
    if (context.mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(peopleError(e))));
    }
  }
}

Future<void> reportPerson(BuildContext context, String id) => showDialog<void>(
  context: context,
  builder: (_) => _ReportDialog(userId: id),
);

class _ReportDialog extends ConsumerStatefulWidget {
  final String userId;
  const _ReportDialog({required this.userId});
  @override
  ConsumerState<_ReportDialog> createState() => _ReportDialogState();
}

class _ReportDialogState extends ConsumerState<_ReportDialog> {
  final _details = TextEditingController();
  String _reason = 'spam';
  String? _error;
  bool _busy = false;
  @override
  void dispose() {
    _details.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(peopleRemoteProvider)
          .report(widget.userId, _reason, _details.text.trim());
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('Report submitted')));
        Navigator.pop(context);
      }
    } catch (e) {
      if (mounted) setState(() => _error = peopleError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_busy,
    child: AlertDialog(
      title: const Text('Report user'),
      content: SizedBox(
        width: 400,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              DropdownButtonFormField<String>(
                initialValue: _reason,
                isExpanded: true,
                decoration: const InputDecoration(labelText: 'Reason'),
                items: const [
                  DropdownMenuItem(value: 'spam', child: Text('Spam')),
                  DropdownMenuItem(
                    value: 'harassment',
                    child: Text('Harassment'),
                  ),
                  DropdownMenuItem(
                    value: 'nudity',
                    child: Text('Sexual content'),
                  ),
                  DropdownMenuItem(value: 'violence', child: Text('Violence')),
                  DropdownMenuItem(value: 'fraud', child: Text('Fraud')),
                  DropdownMenuItem(value: 'other', child: Text('Other')),
                ],
                onChanged: _busy
                    ? null
                    : (value) => setState(() => _reason = value!),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: _details,
                enabled: !_busy,
                maxLength: 1000,
                maxLines: 4,
                decoration: const InputDecoration(
                  labelText: 'Details (optional)',
                  border: OutlineInputBorder(),
                ),
              ),
              const Text(
                'Your account ID, reason, and these details will be sent to BharatChat. Chat messages are not attached.',
              ),
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Text(
                    _error!,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton.icon(
          onPressed: _busy ? null : _send,
          icon: const Icon(Icons.flag_outlined),
          label: Text(_busy ? 'Submitting...' : 'Submit report'),
        ),
      ],
    ),
  );
}
