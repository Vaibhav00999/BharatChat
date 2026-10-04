import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../auth/presentation/providers/auth_providers.dart';
import '../domain/privacy_dashboard.dart';
import 'privacy_providers.dart';

class PrivacyScreen extends ConsumerStatefulWidget {
  const PrivacyScreen({super.key});

  @override
  ConsumerState<PrivacyScreen> createState() => _PrivacyScreenState();
}

class _PrivacyScreenState extends ConsumerState<PrivacyScreen> {
  CancelToken? _exportCancellation;
  bool _deleting = false;

  @override
  void dispose() {
    _exportCancellation?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final dashboard = ref.watch(privacyControllerProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Privacy')),
      body: dashboard.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (_, __) => Center(
          child: FilledButton.icon(
            onPressed: () =>
                ref.read(privacyControllerProvider.notifier).refresh(),
            icon: const Icon(Icons.refresh),
            label: const Text('Retry'),
          ),
        ),
        data: (data) => ListView(
          children: [
            ListTile(
              leading: const Icon(Icons.person_outline),
              title: const Text('Edit profile'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push('/profile/edit'),
            ),
            ListTile(
              leading: const Icon(Icons.block),
              title: const Text('Blocked users'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push('/privacy/blocked'),
            ),
            const _SectionLabel('Visibility'),
            _PrivacyLevelTile(
              title: 'Last seen',
              value: data.settings.lastSeen,
              onChanged: (value) => _update(ref, {'lastSeen': value}),
            ),
            _PrivacyLevelTile(
              title: 'Profile photo',
              value: data.settings.avatar,
              onChanged: (value) => _update(ref, {'avatar': value}),
            ),
            _PrivacyLevelTile(
              title: 'About',
              value: data.settings.about,
              onChanged: (value) => _update(ref, {'about': value}),
            ),
            _PrivacyLevelTile(
              title: 'Phone number',
              value: data.settings.phone,
              onChanged: (value) => _update(ref, {'phone': value}),
            ),
            _PrivacyLevelTile(
              title: 'Group invitations',
              value: data.settings.allowGroupAdds,
              onChanged: (value) => _update(ref, {'allowGroupAdds': value}),
            ),
            SwitchListTile(
              title: const Text('Phone number discovery'),
              value: data.settings.discoverableByPhone,
              onChanged: (value) =>
                  _update(ref, {'discoverableByPhone': value}),
            ),
            SwitchListTile(
              title: const Text('Read receipts'),
              value: data.settings.readReceipts,
              onChanged: (value) => _update(ref, {'readReceipts': value}),
            ),
            SwitchListTile(
              title: const Text('Typing indicators'),
              value: data.settings.shareTypingIndicators,
              onChanged: (value) =>
                  _update(ref, {'shareTypingIndicators': value}),
            ),
            SwitchListTile(
              title: const Text('Security alerts'),
              value: data.settings.securityNotifications,
              onChanged: (value) =>
                  _update(ref, {'securityNotifications': value}),
            ),
            const Divider(),
            const _SectionLabel('Linked devices'),
            for (final device in data.devices)
              _DeviceTile(
                device: device,
                onRemove: device.isCurrent
                    ? null
                    : () => _revoke(context, ref, device),
              ),
            const Divider(),
            Builder(
              builder: (tileContext) => ListTile(
                leading: const Icon(Icons.download_outlined),
                title: Text(
                  _exportCancellation == null
                      ? 'Export account data'
                      : 'Preparing export',
                ),
                onTap: _exportCancellation == null && !_deleting
                    ? () => _exportAccount(tileContext)
                    : null,
                trailing: _exportCancellation == null
                    ? null
                    : IconButton(
                        tooltip: 'Cancel export',
                        onPressed: () => _exportCancellation?.cancel(),
                        icon: const Icon(Icons.close),
                      ),
              ),
            ),
            ListTile(
              leading: Icon(
                Icons.delete_forever_outlined,
                color: Theme.of(context).colorScheme.error,
              ),
              title: Text(
                'Delete account',
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
              onTap: _deleting || _exportCancellation != null
                  ? null
                  : () => _deleteAccount(context, ref),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _exportAccount(BuildContext tileContext) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Export account data?'),
        content: const Text(
          'This archive contains personal data and is not password-protected. Save it only somewhere you trust. It is not a message-key backup.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Export'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted || !tileContext.mounted) return;
    final cancellation = CancelToken();
    final source = ref.read(privacyRemoteDataSourceProvider);
    final save = ref.read(accountExportSaverProvider);
    final userId = ref.read(authControllerProvider).userId;
    setState(() => _exportCancellation = cancellation);
    try {
      final bytes = await source.exportAccount(cancelToken: cancellation);
      if (!mounted ||
          !tileContext.mounted ||
          cancellation.isCancelled ||
          ref.read(authControllerProvider).userId != userId) {
        return;
      }
      final box = tileContext.findRenderObject() as RenderBox?;
      final origin = box == null
          ? const Rect.fromLTWH(0, 0, 1, 1)
          : box.localToGlobal(Offset.zero) & box.size;
      final filename =
          'bharatchat-account-export-${DateTime.now().toUtc().microsecondsSinceEpoch}.zip';
      await save(bytes, filename, origin);
    } catch (error) {
      if (mounted && !cancellation.isCancelled) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              error is DioException && error.response?.statusCode == 429
                  ? 'Export limit reached. Try again in an hour.'
                  : 'Could not export account data. Please retry.',
            ),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _exportCancellation = null);
    }
  }

  Future<void> _update(WidgetRef ref, Map<String, dynamic> value) =>
      ref.read(privacyControllerProvider.notifier).update(value);

  Future<void> _revoke(
    BuildContext context,
    WidgetRef ref,
    LinkedDevice device,
  ) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Unlink device?'),
        content: Text(device.deviceName ?? device.platform),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Unlink'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;
    try {
      await ref
          .read(privacyControllerProvider.notifier)
          .revokeDevice(device.deviceId);
    } catch (_) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Could not unlink device')),
        );
      }
    }
  }

  Future<void> _deleteAccount(BuildContext context, WidgetRef ref) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => const _DeleteAccountDialog(),
    );
    if (confirmed != true || !mounted) return;
    final userId = ref.read(authControllerProvider).userId;
    final keys = ref.read(deviceKeyManagerProvider);
    final auth = ref.read(authControllerProvider.notifier);
    setState(() => _deleting = true);
    try {
      await ref.read(privacyControllerProvider.notifier).deleteAccount();
      try {
        if (userId != null) await keys.clearLocalIdentity(userId);
      } finally {
        await auth.logout();
      }
    } catch (error) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              error is DioException && error.response?.statusCode == 409
                  ? 'Transfer group ownership before deleting your account.'
                  : 'Could not delete account',
            ),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _deleting = false);
    }
  }
}

class _DeleteAccountDialog extends StatefulWidget {
  const _DeleteAccountDialog();
  @override
  State<_DeleteAccountDialog> createState() => _DeleteAccountDialogState();
}

class _DeleteAccountDialogState extends State<_DeleteAccountDialog> {
  String _confirmation = '';
  @override
  Widget build(BuildContext context) => AlertDialog(
    title: const Text('Delete account?'),
    content: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Text(
          'Your account and sessions will be removed. This cannot erase copies other people have already saved. Export your data first if you need it.',
        ),
        const SizedBox(height: 16),
        TextField(
          decoration: const InputDecoration(labelText: 'Type DELETE'),
          onChanged: (value) => setState(() => _confirmation = value.trim()),
        ),
      ],
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.pop(context, false),
        child: const Text('Cancel'),
      ),
      FilledButton(
        onPressed: _confirmation == 'DELETE'
            ? () => Navigator.pop(context, true)
            : null,
        child: const Text('Delete'),
      ),
    ],
  );
}

class _SectionLabel extends StatelessWidget {
  final String text;
  const _SectionLabel(this.text);

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.fromLTRB(16, 20, 16, 8),
    child: Text(text, style: Theme.of(context).textTheme.titleSmall),
  );
}

class _PrivacyLevelTile extends StatelessWidget {
  final String title;
  final String value;
  final ValueChanged<String> onChanged;

  const _PrivacyLevelTile({
    required this.title,
    required this.value,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) => ListTile(
    title: Text(title),
    trailing: DropdownButton<String>(
      value: value,
      underline: const SizedBox.shrink(),
      items: const [
        DropdownMenuItem(value: 'everyone', child: Text('Everyone')),
        DropdownMenuItem(value: 'contacts', child: Text('Contacts')),
        DropdownMenuItem(value: 'nobody', child: Text('Nobody')),
      ],
      onChanged: (next) {
        if (next != null && next != value) onChanged(next);
      },
    ),
  );
}

class _DeviceTile extends StatelessWidget {
  final LinkedDevice device;
  final VoidCallback? onRemove;

  const _DeviceTile({required this.device, this.onRemove});

  @override
  Widget build(BuildContext context) => ListTile(
    leading: Icon(_platformIcon(device.platform)),
    title: Text(device.deviceName ?? device.platform),
    subtitle: Text(
      device.isCurrent
          ? 'This device'
          : device.hasKeyBundle
          ? 'Keys ready'
          : 'Key setup pending',
    ),
    trailing: onRemove == null
        ? const Icon(Icons.verified_user_outlined)
        : IconButton(
            tooltip: 'Unlink device',
            onPressed: onRemove,
            icon: const Icon(Icons.link_off),
          ),
  );

  IconData _platformIcon(String platform) => switch (platform) {
    'android' => Icons.android,
    'ios' => Icons.phone_iphone,
    'web' => Icons.language,
    _ => Icons.computer,
  };
}
