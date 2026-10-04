import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../chat_list/presentation/providers/chat_list_providers.dart';
import '../providers/group_providers.dart';

class CreateGroupScreen extends ConsumerStatefulWidget {
  const CreateGroupScreen({super.key});

  @override
  ConsumerState<CreateGroupScreen> createState() => _CreateGroupScreenState();
}

class _CreateGroupScreenState extends ConsumerState<CreateGroupScreen> {
  final _form = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _description = TextEditingController();
  final _members = TextEditingController();
  bool _saving = false;

  @override
  void dispose() {
    _name.dispose();
    _description.dispose();
    _members.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_form.currentState!.validate()) return;

    final ids = _members.text
        .split(RegExp(r'[\s,]+'))
        .where((id) => id.isNotEmpty)
        .toSet()
        .toList();
    setState(() => _saving = true);

    try {
      final group = await ref
          .read(groupRemoteDataSourceProvider)
          .create(
            name: _name.text.trim(),
            description: _description.text.trim(),
            memberIds: ids,
          );
      await ref.read(chatListProvider.notifier).refresh();
      if (mounted) {
        context.go(
          '/chat/${group.chatId}?title=${Uri.encodeQueryComponent(group.name)}&group=true',
        );
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('Could not create group')));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('New group')),
      body: SafeArea(
        child: Form(
          key: _form,
          child: ListView(
            padding: const EdgeInsets.all(24),
            children: [
              TextFormField(
                controller: _name,
                maxLength: 120,
                decoration: const InputDecoration(labelText: 'Group name'),
                validator: (value) => value == null || value.trim().isEmpty
                    ? 'Group name is required'
                    : null,
              ),
              const SizedBox(height: 12),
              TextFormField(
                controller: _description,
                maxLength: 500,
                maxLines: 3,
                decoration: const InputDecoration(labelText: 'Description'),
              ),
              const SizedBox(height: 12),
              TextFormField(
                controller: _members,
                minLines: 3,
                maxLines: 6,
                decoration: const InputDecoration(
                  labelText: 'Member user IDs',
                  helperText: 'Separate IDs with commas or spaces',
                ),
                validator: (value) => value == null || value.trim().isEmpty
                    ? 'Add at least one member'
                    : null,
              ),
              const SizedBox(height: 20),
              FilledButton.icon(
                onPressed: _saving ? null : _submit,
                icon: const Icon(Icons.group_add),
                label: const Text('Create group'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
